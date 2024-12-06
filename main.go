package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

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

// Middleware to ensure the user is logged in.
func middlewareLoggedIn(handler func(s *state, cmd command, user uuid.UUID) error) func(*state, command) error {
	return func(s *state, cmd command) error {
		userUUID, err := s.db.GetUserUUID(context.Background(), s.config.Name)
		if err != nil {
			return fmt.Errorf("could not find UUID for user '%s', error: %v", s.config.Name, err)
		}

		return handler(s, cmd, userUUID)
	}
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

// parseFeedDate attempts to parse RSS feed dates in various formats
func parseFeedDate(dateStr string) (time.Time, error) {
	layouts := []string{
		time.RFC1123Z,
		time.RFC1123,
		time.RFC822Z,
		time.RFC822,
		"2006-01-02T15:04:05Z07:00",
		"Mon, 02 Jan 2006 15:04:05 -0700",
		"02 Jan 2006 15:04:05 -0700",
		"2006-01-02 15:04:05",
	}

	for _, layout := range layouts {
		if t, err := time.Parse(layout, dateStr); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unable to parse date: %s", dateStr)
}

func scrapeFeeds(s *state) error {
	ctx := context.Background()

	// Get the next feed to fetch (the one with oldest or null last_fetched_at)
	feedURL, err := s.db.GetNextFeedToFetch(context.Background(), s.config.Name)
	if err != nil {
		return fmt.Errorf("failed to get next feed: %w", err)
	}

	// Fetch the feed content
	rssFeed, err := fetchFeed(ctx, feedURL)
	if err != nil {
		return fmt.Errorf("failed to fetch feed %s: %w", feedURL, err)
	}

	feedNameAndID, err := s.db.GetFeedNamebyURL(context.Background(), feedURL)
	if err != nil {
		return fmt.Errorf("failed to fetch feed name for url, consider adding the feed first ...: %v", err)
	}

	// Process and save each post
	for _, item := range rssFeed.Channel.Item {
		// Parse the publication date
		pubDate, err := parseFeedDate(item.PubDate)
		if err != nil {
			fmt.Printf("Warning: couldn't parse date for post '%s': %v\n", item.Title, err)
			// Use current time as fallback
			pubDate = time.Now()
		}

		now := time.Now()
		_, err = s.db.CreatePost(ctx, database.CreatePostParams{
			ID:          uuid.New(),
			CreatedAt:   now,
			UpdatedAt:   now,
			Title:       item.Title,
			Url:         item.Link,
			Description: item.Description,
			PublishedAt: pubDate,
			FeedID:      feedNameAndID.ID,
		})
		if err != nil {
			// Check if it's a uniqueness violation
			if strings.Contains(err.Error(), "unique constraint") {
				continue // Skip duplicates silently
			}
			fmt.Printf("Error saving post '%s': %v\n", item.Title, err)
			continue
		}
	}

	// Print feed items
	fmt.Printf("\nFeed: %s\n", feedNameAndID.Name)
	for _, item := range rssFeed.Channel.Item {
		fmt.Printf("- %s\n", item.Title)
	}

	// Mark the feed as fetched
	err = s.db.MarkFeedFetched(context.Background(), database.MarkFeedFetchedParams{
		LastFetchedAt: sql.NullTime{Time: time.Now(), Valid: true},
		UpdatedAt:     time.Now(),
		ID:            feedNameAndID.ID,
	})
	if err != nil {
		return fmt.Errorf("failed to mark feed as fetched: %w", err)
	}

	return nil
}

func handlerAgg(s *state, cmd command) error {
	// ctx := context.Background()
	// feedURL := "https://www.wagslane.dev/index.xml" //this needs to change, no hardcoding

	// // Fetch the RSS feed
	// rssFeed, err := fetchFeed(ctx, feedURL)
	// if err != nil {
	// 	return fmt.Errorf("failed to fetch feed: %w", err)
	// }

	// // Print the RSSFeed struct
	// fmt.Printf("Fetched RSS Feed:\n%+v\n", rssFeed)
	// return nil

	if len(cmd.args) < 1 {
		return fmt.Errorf("agg command requires time_between_reqs parameter (e.g. '1m', '30s')")
	}

	// Parse the duration string
	timeBetweenRequests, err := time.ParseDuration(cmd.args[0])
	if err != nil {
		return fmt.Errorf("invalid duration format: %v", err)
	}

	fmt.Printf("Collecting feeds every %v\n", timeBetweenRequests)

	// Create a ticker for periodic execution
	ticker := time.NewTicker(timeBetweenRequests)
	defer ticker.Stop()

	// Run immediately and then on every tick
	for ; ; <-ticker.C {
		err := scrapeFeeds(s)
		if err != nil {
			fmt.Printf("Error scraping feeds: %v\n", err)
			// Continue running even if there's an error
			continue
		}
	}
}

// Add the browse command handler
func handlerBrowse(s *state, cmd command) error {
	limit := 2 // Default limit
	if len(cmd.args) > 0 {
		parsedLimit, err := strconv.Atoi(cmd.args[0])
		if err != nil {
			return fmt.Errorf("invalid limit parameter: %v", err)
		}
		limit = parsedLimit
	}

	posts, err := s.db.GetPostsForUser(context.Background(), database.GetPostsForUserParams{
		Name:  s.config.Name,
		Limit: int32(limit),
	})
	if err != nil {
		return fmt.Errorf("failed to fetch posts: %v", err)
	}

	if len(posts) == 0 {
		fmt.Println("No posts found.")
		return nil
	}

	fmt.Printf("Latest %d posts:\n\n", limit)

	for _, post := range posts {
		fmt.Println("-----------------------------")
		fmt.Printf("%s - %s (%s)\n", post.Name, post.Title, post.PublishedAt.Format("2006-01-02 15:04:05"))
		fmt.Println("*****************************")
		fmt.Printf("%s\n", post.Description)
		fmt.Println("-----------------------------")
	}
	/*
		fmt.Printf("Latest %d posts:\n\n", limit)
		for _, post := range posts {
			fmt.Printf("Title: %s\n", post.Title)
			fmt.Printf("URL: %s\n", post.Url)
			if post.Description.Valid {
				fmt.Printf("Description: %s\n", post.Description.String)
			}
			if post.PublishedAt.Valid {
				fmt.Printf("Published: %s\n", post.PublishedAt.Time.Format("2006-01-02 15:04:05"))
			}
			fmt.Println("---")
		}
	*/
	return nil
}

func handlerAddFeed(s *state, cmd command, userUUID uuid.UUID) error {
	if len(cmd.args) < 2 {
		return fmt.Errorf("addfeed command requires the name of the feed and URL of the feed")
	}

	feedName := cmd.args[0]
	feedURL := cmd.args[1]

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
	feedID := uuid.New()
	_, err = s.db.AddFeed(context.Background(), database.AddFeedParams{
		ID:            feedID,
		CreatedAt:     now,
		UpdatedAt:     now,
		Name:          feedName,
		Url:           feedURL,
		LastFetchedAt: sql.NullTime{},
		UserID:        userUUID,
	})
	if err != nil {
		return fmt.Errorf("failed to create feed entry: %v", err)
	}

	// Add the new feed in feed_followed
	_, err = s.db.CreateFeedFollow(context.Background(), database.CreateFeedFollowParams{
		ID:        uuid.New(),
		CreatedAt: now,
		UpdatedAt: now,
		UserID:    userUUID,
		FeedID:    feedID,
	})
	if err != nil {
		return fmt.Errorf("failed to follow feed: %v", err)
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

func handlerFollowFeeds(s *state, cmd command, userUUID uuid.UUID) error {
	if len(cmd.args) < 1 {
		return fmt.Errorf("follow command requires the URL of the feed")
	}
	feedURL := cmd.args[0]

	feedNameAndID, err := s.db.GetFeedNamebyURL(context.Background(), feedURL)
	if err != nil {
		return fmt.Errorf("failed to fetch feed name for url, consider adding the feed first ...: %v", err)
	}

	// Add the new feed in feed_followed
	now := time.Now()
	_, err = s.db.CreateFeedFollow(context.Background(), database.CreateFeedFollowParams{
		ID:        uuid.New(),
		CreatedAt: now,
		UpdatedAt: now,
		UserID:    userUUID,
		FeedID:    feedNameAndID.ID,
	})
	if err != nil {
		return fmt.Errorf("failed to follow feed: %v", err)
	}

	fmt.Printf("Now following feed '%s' ('%s').\n", feedNameAndID.Name, feedURL)
	return nil
}

func handlerFollowingFeeds(s *state, cmd command, userUUID uuid.UUID) error {

	folowedFeedList, err := s.db.GetFeedFollowsForUser(context.Background(), s.config.Name)
	if err != nil {
		return fmt.Errorf("failed to fetch feeds: %v", err)
	}
	fmt.Println("Feed names")
	for _, feedname := range folowedFeedList {
		fmt.Printf("* '%s'\n", feedname.Name)
	}

	return nil
}

func handlerUnfollowFeeds(s *state, cmd command, userUUID uuid.UUID) error {
	if len(cmd.args) < 1 {
		return fmt.Errorf("follow command requires the URL of the feed")
	}
	feedURL := cmd.args[0]

	var userNameFeedID database.GetFeedIDUserIDfromFollowsParams
	userNameFeedID.Url = feedURL
	userNameFeedID.Name = s.config.Name

	feedNameAndID, err := s.db.GetFeedIDUserIDfromFollows(context.Background(), userNameFeedID)
	if err != nil {
		return fmt.Errorf("failed to fetch feed name for url: %v", err)
	}

	var feedIDnameID database.DeleteFeedFollowParams
	feedIDnameID.FeedID = feedNameAndID.FeedID
	feedIDnameID.UserID = feedNameAndID.UserID

	err = s.db.DeleteFeedFollow(context.Background(), feedIDnameID)
	if err != nil {
		return fmt.Errorf("failed to unfollow feed: %v", err)
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
	cmds.register("addfeed", middlewareLoggedIn(handlerAddFeed))
	cmds.register("feeds", handlerListFeeds)
	cmds.register("follow", middlewareLoggedIn(handlerFollowFeeds))
	cmds.register("following", middlewareLoggedIn(handlerFollowingFeeds))
	cmds.register("unfollow", middlewareLoggedIn(handlerUnfollowFeeds))
	cmds.register("browse", handlerBrowse)

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
