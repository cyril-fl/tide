package config

import (
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"strings"

	plumcliconfig "github.com/cyril-fl/plum/cli/config"
	plummillload "github.com/cyril-fl/plum/services/factory/millload"
	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

// version is set by the images (-ldflags -X), whose build context has no .git to read the commit from.
var version string

// Version comes from the build (the tag of an image, the git commit, or module version without one): nothing to keep
// in sync by hand. tide and tide-server of one build answer the same.
func Version() string {
	if version != "" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			return setting.Value
		}
	}
	return info.Main.Version
}

// embedded is the config built into the binary, so it runs from any folder; CONFIG_FILE overrides it.
//
//go:embed config.yaml
var embedded []byte

var Current config

type config struct {
	Name     string   `yaml:"name"`
	Commands commands `yaml:"commands"`
}

type commands struct {
	Create            plumcliconfig.Cmdparams `yaml:"create"`
	CreateNote        plumcliconfig.Cmdparams `yaml:"create-note"`
	CreateFolder      plumcliconfig.Cmdparams `yaml:"create-folder"`
	CreateTag         plumcliconfig.Cmdparams `yaml:"create-tag"`
	CreateToken       plumcliconfig.Cmdparams `yaml:"create-token"`
	Read              plumcliconfig.Cmdparams `yaml:"read"`
	ReadNote          plumcliconfig.Cmdparams `yaml:"read-note"`
	ReadFolder        plumcliconfig.Cmdparams `yaml:"read-folder"`
	ReadLinks         plumcliconfig.Cmdparams `yaml:"read-links"`
	ReadTag           plumcliconfig.Cmdparams `yaml:"read-tag"`
	ReadToken         plumcliconfig.Cmdparams `yaml:"read-token"`
	Edit              plumcliconfig.Cmdparams `yaml:"edit"`
	EditNote          plumcliconfig.Cmdparams `yaml:"edit-note"`
	EditFolder        plumcliconfig.Cmdparams `yaml:"edit-folder"`
	EditTag           plumcliconfig.Cmdparams `yaml:"edit-tag"`
	Delete            plumcliconfig.Cmdparams `yaml:"delete"`
	DeleteNote        plumcliconfig.Cmdparams `yaml:"delete-note"`
	DeleteFolder      plumcliconfig.Cmdparams `yaml:"delete-folder"`
	DeleteTag         plumcliconfig.Cmdparams `yaml:"delete-tag"`
	DeleteToken       plumcliconfig.Cmdparams `yaml:"delete-token"`
	Move              plumcliconfig.Cmdparams `yaml:"move"`
	Show              plumcliconfig.Cmdparams `yaml:"show"`
	ShowFinder        plumcliconfig.Cmdparams `yaml:"show-finder"`
	ShowGraph         plumcliconfig.Cmdparams `yaml:"show-graph"`
	Trash             plumcliconfig.Cmdparams `yaml:"trash"`
	Restore           plumcliconfig.Cmdparams `yaml:"restore"`
	Replace           plumcliconfig.Cmdparams `yaml:"replace"`
	Search            plumcliconfig.Cmdparams `yaml:"search"`
	Tag               plumcliconfig.Cmdparams `yaml:"tag"`
	TagNotes          plumcliconfig.Cmdparams `yaml:"tag-notes"`
	Recent            plumcliconfig.Cmdparams `yaml:"recent"`
	Export            plumcliconfig.Cmdparams `yaml:"export"`
	Import            plumcliconfig.Cmdparams `yaml:"import"`
	Serve             plumcliconfig.Cmdparams `yaml:"serve"`
	MCP               plumcliconfig.Cmdparams `yaml:"mcp"`
	Login             plumcliconfig.Cmdparams `yaml:"login"`
	Logout            plumcliconfig.Cmdparams `yaml:"logout"`
	Whoami            plumcliconfig.Cmdparams `yaml:"whoami"`
	User              plumcliconfig.Cmdparams `yaml:"user"`
	UserAdd           plumcliconfig.Cmdparams `yaml:"user-add"`
	UserPerf          plumcliconfig.Cmdparams `yaml:"user-perf"`
	UserResetPassword plumcliconfig.Cmdparams `yaml:"user-reset-password"`
	RotateSecret      plumcliconfig.Cmdparams `yaml:"rotate-secret"`
	Account           plumcliconfig.Cmdparams `yaml:"account"`
	AccountPurge      plumcliconfig.Cmdparams `yaml:"account-purge"`
	Root              plumcliconfig.Cmdparams `yaml:"root"`
	ServerRoot        plumcliconfig.Cmdparams `yaml:"server-root"`
}

func load() (*config, error) {
	file := strings.TrimSpace(os.Getenv("CONFIG_FILE"))
	if file == "" {
		var current config
		if err := yaml.Unmarshal(embedded, &current); err != nil {
			return nil, fmt.Errorf("embedded config: %w", err)
		}
		return &current, nil
	}

	loader := plummillload.New(file)
	current := loader.LoadFromFile[config]()

	if loader.HasErr() {
		return nil, fmt.Errorf("%w", loader.GetErr())
	}

	return current, nil
}

// Env is the name of the app's environment variable key: TIDE_<key>, the prefix from the name.
// TODO: NOTES_<key> is read while TIDE_<key> is unset, remove it once the server's .env is migrated to TIDE_.
func Env(key string) string {
	name := strings.ToUpper(Current.Name) + "_" + key
	if _, set := os.LookupEnv(name); !set {
		if _, set := os.LookupEnv("NOTES_" + key); set {
			return "NOTES_" + key
		}
	}
	return name
}

// Title is the name of the app as shown: Tide.
func Title() string {
	return strings.ToUpper(Current.Name[:1]) + Current.Name[1:]
}

// Dir is the user's folder of the app, ~/.config/tide: its session and its .env.
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	return filepath.Join(home, ".config", Current.Name), err
}

// File is the file of Dir to read by that name, or the one of ~/.config/notes when Dir has none. Writes go to Dir.
// TODO: ~/.config/notes is read while ~/.config/tide lacks the file, remove it once the server is migrated to Tide.
func File(name string) (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	file := filepath.Join(dir, name)
	if _, err := os.Stat(file); errors.Is(err, fs.ErrNotExist) {
		old := filepath.Join(filepath.Dir(dir), "notes", name)
		if _, err := os.Stat(old); err == nil {
			return old, nil
		}
	}
	return file, nil
}

// expand writes the name into the texts of the commands, which say {name} for the program and {NAME} for the prefix
// of its environment variables.
func (c *config) expand() {
	replacer := strings.NewReplacer("{name}", c.Name, "{NAME}", strings.ToUpper(c.Name))
	commands := reflect.ValueOf(&c.Commands).Elem()
	for i := range commands.NumField() {
		params := commands.Field(i).Addr().Interface().(*plumcliconfig.Cmdparams)
		params.Use, params.Short, params.Long, params.Example = replacer.Replace(params.Use), replacer.Replace(params.Short), replacer.Replace(params.Long), replacer.Replace(params.Example)
		for j := range params.Flags {
			params.Flags[j].Usage = replacer.Replace(params.Flags[j].Usage)
		}
	}
}

// loadEnv reads the user file first: godotenv never overwrites a value, so the .env of whatever
// project the CLI runs from cannot point it at another database.
func loadEnv() {
	if file, err := File(".env"); err == nil {
		godotenv.Load(file)
	}
	godotenv.Load()
}

func init() {
	// The name of the user's folder, for loadEnv: the CONFIG_FILE it may set is read after.
	if err := yaml.Unmarshal(embedded, &Current); err != nil {
		panic(fmt.Errorf("embedded config: %w", err))
	}
	loadEnv()
	loaded, err := load()
	if err != nil {
		panic(err)
	}
	Current = *loaded
	Current.expand()
}
