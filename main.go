package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"context"
	"time"

	"encoding/xml"
	"html"
	"io"
	"net/http"

	"github.com/1729prashant/blog-aggregator/internal/config"
	"github.com/1729prashant/blog-aggregator/internal/database"
	"github.com/google/uuid"
	_ "github.com/lib/pq" // ?? You have to import the driver, but you don't use it directly anywhere in your code. The underscore tells Go that you're importing it for its side effects, not because you need to use it.
)

type state struct {
	db     *database.Queries
	config *config.Config
}

type command struct {
	name string
	args []string
}

type commands struct {
	cmds map[string]func(*state, command) error
}

// Register a new handler function for a command name.
func (c *commands) register(name string, f func(*state, command) error) {
	if c.cmds == nil {
		c.cmds = make(map[string]func(*state, command) error)
	}
	c.cmds[name] = f
}

// Run a given command with the provided state if it exists.
func (c *commands) run(s *state, cmd command) error {
	handler, exists := c.cmds[cmd.name]
	if !exists {
		return fmt.Errorf("unknown command: %s", cmd.name)
	}
	return handler(s, cmd)
}

// Login handler: sets the current user in the config file.
func handlerLogin(s *state, cmd command) error {
	if len(cmd.args) < 1 {
		return fmt.Errorf("login command requires a username")
	}

	username := cmd.args[0]

	// Check if the user exists
	existingUser, err := s.db.GetUser(context.Background(), username)
	if err != nil {
		return fmt.Errorf("user '%s' is not registered: %v", username, err)
	}

	if existingUser != username {
		return fmt.Errorf("user '%s' is not registered", username)
	}

	// Set the user in the config
	err = s.config.SetUser(username)
	if err != nil {
		return fmt.Errorf("failed to set user: %v", err)
	}

	fmt.Printf("User '%s' has successfully logged in.\n", username)
	return nil
}

func handlerRegister(s *state, cmd command) error {
	if len(cmd.args) < 1 {
		return fmt.Errorf("register command requires a username")
	}

	username := cmd.args[0]

	// Check if the user already exists
	existingUser, err := s.db.GetUser(context.Background(), username)
	if err == nil && existingUser == username {
		return fmt.Errorf("User '%s' already exists.\n", username)
	}

	// Create a new user
	now := time.Now()
	newUser, err := s.db.CreateUser(context.Background(), database.CreateUserParams{
		ID:        uuid.New(),
		Name:      username,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		return fmt.Errorf("failed to create user: %v", err)
	}

	// Update the current user in the config
	err = s.config.SetUser(newUser.Name)
	if err != nil {
		return fmt.Errorf("failed to set user in config: %v", err)
	}

	fmt.Printf("User '%s' created successfully: %+v\n", username, newUser)
	return nil
}

func ResetAllUsers(s *state, cmd command) error {
	err := s.db.ResetUsers(context.Background())
	if err != nil {
		return fmt.Errorf("failed to clear the user table: %v", err)
	}

	return nil
}

func GetUsers(s *state, cmd command) error {
	userList, err := s.db.GetUsers(context.Background())
	if err != nil {
		return fmt.Errorf("failed to fetch users: %v", err)
	}

	for _, userName := range userList {
		if userName == s.config.Name {
			fmt.Printf("* '%s (current)'\n", userName)
		} else {
			fmt.Printf("* '%s'\n", userName)
		}
	}

	return nil
}

type RSSFeed struct {
	Channel struct {
		Title       string    `xml:"title"`
		Link        string    `xml:"link"`
		Description string    `xml:"description"`
		Item        []RSSItem `xml:"item"`
	} `xml:"channel"`
}

type RSSItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
}

func fetchFeed(ctx context.Context, feedURL string) (*RSSFeed, error) {
	// Create a new HTTP request with context
	req, err := http.NewRequestWithContext(ctx, "GET", feedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set the User-Agent header
	req.Header.Add("User-Agent", "gator")

	// Execute the request using an HTTP client
	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch RSS: %w", err)
	}
	defer res.Body.Close()

	// Check for non-success HTTP status codes
	if res.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status code: %d", res.StatusCode)
	}

	// Read the response body
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse the XML into an RSSFeed struct
	var response RSSFeed
	err = xml.Unmarshal(body, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Unescape HTML entities in Titles and Descriptions
	response.Channel.Title = html.UnescapeString(response.Channel.Title)
	response.Channel.Description = html.UnescapeString(response.Channel.Description)
	for i := range response.Channel.Item {
		response.Channel.Item[i].Title = html.UnescapeString(response.Channel.Item[i].Title)
		response.Channel.Item[i].Description = html.UnescapeString(response.Channel.Item[i].Description)
	}

	return &response, nil
}

func handlerAgg(s *state, cmd command) error {
	ctx := context.Background()
	feedURL := "https://www.wagslane.dev/index.xml"

	// Fetch the RSS feed
	rssFeed, err := fetchFeed(ctx, feedURL)
	if err != nil {
		return fmt.Errorf("failed to fetch feed: %w", err)
	}

	// Print the RSSFeed struct
	fmt.Printf("Fetched RSS Feed:\n%+v\n", rssFeed)
	return nil
}

func handlerAddFeed(s *state, cmd command) error {
	if len(cmd.args) < 2 {
		return fmt.Errorf("addfeed command requires the name of the feed and URL of the feed")
	}

	feedName := cmd.args[0]
	feedURL := cmd.args[1]

	// Get the current user UUID
	userUUID, err := s.db.GetUserUUID(context.Background(), s.config.Name)
	if err != nil {
		return fmt.Errorf("could not find UUID for user '%s', error: %v", s.config.Name, err)
	}

	// Check if the feed already exists using a combination of feed name and user UUID
	existingFeed, err := s.db.GetFeed(context.Background(), database.GetFeedParams{
		Name:   feedName,
		UserID: userUUID,
	})
	if err == nil && existingFeed == feedName {
		return fmt.Errorf("feed '%s' already exists", feedName)
	}

	if err != nil && err.Error() != "sql: no rows in result set" {
		// Handle database errors except "no rows found"
		return fmt.Errorf("failed to check existing feed: %v", err)
	}

	// Add the new feed
	now := time.Now()
	_, err = s.db.AddFeed(context.Background(), database.AddFeedParams{
		ID:        uuid.New(),
		CreatedAt: now,
		UpdatedAt: now,
		Name:      feedName,
		Url:       feedURL,
		UserID:    userUUID,
	})
	if err != nil {
		return fmt.Errorf("failed to create feed entry: %v", err)
	}

	fmt.Printf("Feed '%s' successfully added.\n", feedName)
	return nil
}

func handlerListFeeds(s *state, cmd command) error {
	feedList, err := s.db.GetAllFeeds(context.Background())
	if err != nil {
		return fmt.Errorf("failed to fetch feeds: %v", err)
	}
	fmt.Println("Feed name, URL, User Name")
	for _, feedname := range feedList {
		fmt.Printf("'%s', '%s', '%s'\n", feedname.Name, feedname.Url, feedname.Name_2)
	}

	return nil
}

func main() {
	// Load the configuration
	cfg, err := config.Read()
	if err != nil {
		log.Fatalf("Failed to read config: %v", err)
	}

	// Open the database connection
	db, err := sql.Open("postgres", cfg.DbURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Initialize database queries
	dbQueries := database.New(db)

	appState := &state{
		config: &cfg,
		db:     dbQueries,
	}

	// Initialize the commands
	cmds := &commands{}
	cmds.register("login", handlerLogin)
	cmds.register("register", handlerRegister)
	cmds.register("reset", ResetAllUsers)
	cmds.register("users", GetUsers)
	cmds.register("agg", handlerAgg)
	cmds.register("addfeed", handlerAddFeed)
	cmds.register("feeds", handlerListFeeds)

	// Parse the command-line arguments
	if len(os.Args) < 2 {
		fmt.Println("Error: not enough arguments provided.")
		os.Exit(1)
	}

	cmd := command{
		name: os.Args[1],
		args: os.Args[2:],
	}

	// Run the command
	err = cmds.run(appState, cmd)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

}
