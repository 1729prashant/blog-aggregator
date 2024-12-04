package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"context"
	"time"

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
