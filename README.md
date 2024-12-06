# blog-aggregator
--
originally based on boot-dev project, but undergoing several changes ...


#### pre-requisites
1. install go <https://go.dev/doc/install>

2. Install Postgres v15 or later and create config file
> Mac OS with brew
> ```
> brew install postgresql@15
> ```
> 
> Linux / WSL (Debian). Here are the [docs from Microsoft](https://learn.microsoft.com/en-us/windows/wsl/tutorials/wsl-database#install-postgresql), but simply:
> 
> ```
> sudo apt update
> sudo apt install postgresql postgresql-contrib
> ```
> 
> Ensure the installation worked. The psql command-line utility is the default client for Postgres. Use it to make sure you're on version 15+ of Postgres:
> 
> ```
> psql --version
> ```
> 
> (Linux only) Update postgres password:
> 
> ```
> sudo passwd postgres
> ```
> 
> Start the Postgres server in the background
> Mac: brew services start postgresql
> Linux: sudo service postgresql start
> 
> Connect to the server.
> Enter the psql shell:
> > 
> > Mac: psql postgres
> > 
> > Linux: sudo -u postgres psql
> > 
> > You should see a new prompt that looks like this:
> > ```
> > postgres=#
> > ```
> 
> Create a new database.
> ```
> CREATE DATABASE gator;
> ```
> 
> Connect to the new database:
> ```
> \c gator
> ```
> 
> >You should see a new prompt that looks like this:
> >```
> >gator=#
> >```
> 
> Set the user password (Linux only)
> ```
> ALTER USER postgres PASSWORD 'your password';
> ```
> 
> Run the following, it should return the latest version. If everything is working, you can move on. You can type exit to leave the psql shell.
> 
> ```
> SELECT version();
> ```
> 
> 
> 
> 
> Install Goose
> 
> ```
> go install github.com/pressly/goose/v3/cmd/goose@latest
> ```
> 
> 
> Get your connection string. A connection string is just a URL with all of the information needed to connect to a database. The format is:
> 
> ```
> protocol://username:password@host:port/database
> ```
> 
> Run the up migration.
> 
> ```
> goose postgres <connection_string> up
> ```
> 
> Add the connection string to the .gatorconfig.json file instead of the example string we used earlier. When using it with goose, you'll use it in the format we just used. However, here in the config file it needs an additional sslmode=disable query string:
> 
> ```
> protocol://username:password@host:port/database?sslmode=disable
> ```
> 
> Install SQLC. (TODO check)
> ```
> go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
> ```



#### installation
clone the repo
```
git clone https://github.com/1729prashant/blog-aggregator
```

and use one of the two options
> Option 1: Move to /usr/local/bin (requires sudo)
> ```
> sudo mv gator /usr/local/bin/
> ```
> 
> Option 2: Move to ~/bin (create directory first if it doesn't exist)
> ```
> mkdir -p ~/bin
> mv gator ~/bin/
> ```
> 
> > If you use Option 2 (~/bin), make sure it's in your PATH by adding this to your ~/.bashrc or ~/.zshrc:
> > 
> > ```
> > export PATH="$HOME/bin:$PATH"
> > ```


Manually create a config file in your home directory, ~/.gatorconfig.json, with the following content:

```
{
  "db_url": "postgres://example",
  "current_user_name":""
}
```


You can then use gator from anywhere:

```
gator login <username>
gator register <username>
gator addfeed <name> <url>
...
```


>>TODO: CUrrently called gator as per project requirement, will change as program is updated.