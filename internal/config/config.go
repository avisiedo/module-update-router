package config

import (
	"flag"
	"fmt"

	clowder "github.com/redhatinsights/app-common-go/pkg/api/v1"
	"github.com/sgreben/flagvar"
)

// Config stores values that are used to configure the application.
type Config struct {
	Addr           string
	APIVersion     string
	AppName        string
	DBDriver       flagvar.Enum
	DBHost         string
	DBName         string
	DBPass         string
	DBPort         int
	DBURL          string
	DBUser         string
	EventBuffer    int
	KafkaBootstrap string
	LogFormat      flagvar.Enum
	LogLevel       string
	MAddr          string
	MetricsTopic   string
	PathPrefix     string
	Reset          bool
	SeedPath       flagvar.File
}

// DefaultConfig is the default configuration variable, providing access to
// configuration values globally.
var DefaultConfig Config = Config{
	Addr:           ":8080",
	APIVersion:     "v1",
	AppName:        "module-update-router",
	DBDriver:       flagvar.Enum{Choices: []string{"pgx", "sqlite3"}, Value: "sqlite3"},
	DBHost:         "localhost",
	DBName:         "postgres",
	DBPass:         "",
	DBPort:         5432,
	DBURL:          "",
	DBUser:         "postgres",
	EventBuffer:    1000,
	KafkaBootstrap: "",
	LogFormat:      flagvar.Enum{Choices: []string{"text", "json"}, Value: "text"},
	LogLevel:       "info",
	MAddr:          ":2112",
	MetricsTopic:   "client-metrics",
	PathPrefix:     "/api",
	Reset:          false,
	SeedPath:       flagvar.File{},
}

// init can be used to set default values for DefaultConfig that require more
// complex computation, such as external package function calls.
func init() {
	if clowder.IsClowderEnabled() {
		DefaultConfig.Addr = fmt.Sprintf(":%v", *clowder.LoadedConfig.PublicPort)
		DefaultConfig.DBHost = clowder.LoadedConfig.Database.Hostname
		DefaultConfig.DBName = clowder.LoadedConfig.Database.Name
		DefaultConfig.DBPass = clowder.LoadedConfig.Database.Password
		DefaultConfig.DBPort = clowder.LoadedConfig.Database.Port
		DefaultConfig.DBUser = clowder.LoadedConfig.Database.Username
		DefaultConfig.MAddr = fmt.Sprintf(":%v", clowder.LoadedConfig.MetricsPort)
	}
}

// FlagSet creates a new FlagSet, defined with flags for each struct field in
// the DefaultConfig variable.
func FlagSet(name string, errorHandling flag.ErrorHandling) *flag.FlagSet {
	fs := flag.NewFlagSet(name, errorHandling)

	fs.Var(&DefaultConfig.DBDriver, "db-driver", fmt.Sprintf("database driver (%v)", DefaultConfig.DBDriver.Help()))
	fs.StringVar(&DefaultConfig.DBHost, "db-host", DefaultConfig.DBHost, "IP or hostname of database server")
	fs.StringVar(&DefaultConfig.DBName, "db-name", DefaultConfig.DBName, "database name")
	fs.StringVar(&DefaultConfig.DBPass, "db-pass", DefaultConfig.DBPass, "database user password")
	fs.IntVar(&DefaultConfig.DBPort, "db-port", DefaultConfig.DBPort, "TCP port on database server")
	fs.StringVar(&DefaultConfig.DBURL, "database-url", DefaultConfig.DBURL, "database connection URL")
	fs.StringVar(&DefaultConfig.DBUser, "db-user", DefaultConfig.DBUser, "database username")
	fs.Var(&DefaultConfig.LogFormat, "log-format", fmt.Sprintf("set logging format (%v)", DefaultConfig.LogFormat.Help()))
	fs.StringVar(&DefaultConfig.LogLevel, "log-level", DefaultConfig.LogLevel, "logging level")

	return fs
}
