package server

import (
	"crypto/subtle"
	"fmt"
	"github.com/philips-labs/cf-crontab/crontab"
	"net/http"
	"os"
	"strconv"
	"sync"

	"github.com/labstack/echo/v4"
	"github.com/robfig/cron/v3"
	"github.com/spf13/viper"
)

// authCheck verifies basic auth. Username hardcoded to `redshift`
func authCheck(username, password string, c echo.Context) (bool, error) {
	if subtle.ConstantTimeCompare([]byte(username), []byte("redshift")) == 1 &&
		subtle.ConstantTimeCompare([]byte(password), []byte(password)) == 1 {
		return true, nil
	}
	return false, nil
}

func entriesDeleteHandler(state *State) echo.HandlerFunc {
	return func(c echo.Context) error {
		stringID := c.Param("entryID")
		entryID, err := strconv.Atoi(stringID)
		if err != nil {
			return err
		}
		err = state.DeleteEntry(entryID)
		if err != nil {
			return err
		}
		return c.String(http.StatusNoContent, "")
	}
}

func entriesGetHandler(state *State) echo.HandlerFunc {
	return func(c echo.Context) error {
		return c.JSONPretty(http.StatusOK, state.Entries(), "  ")
	}
}

func entriesPostHandler(state *State) echo.HandlerFunc {
	return func(c echo.Context) error {
		newEntries := []crontab.Task{}
		if err := c.Bind(&newEntries); err != nil {
			return err
		}
		state.AddEntries(newEntries)
		return c.JSONPretty(http.StatusOK, newEntries, "  ")
	}
}

type State struct {
	list []*crontab.Task
	cronTab *cron.Cron
	mux sync.Mutex
}

func (e *State)Entries() []*crontab.Task{
	return e.list
}

func (e *State)StartCron() {
	e.cronTab.Start()
}

func (e *State)AddEntries(newEntries []crontab.Task) {
	e.mux.Lock()
	defer e.mux.Unlock()
	for i, _ := range newEntries {
		_ = newEntries[i].Add(e.cronTab)
		e.list = append(e.list, &newEntries[i])
	}
}

func (e *State) DeleteEntry(id int) error {
	e.mux.Lock()
	defer e.mux.Unlock()
	entryID := cron.EntryID(id)
	for i, t := range e.Entries() {
		if int(t.EntryID) == id {
			fmt.Printf("Removing %d\n", id)
			e.list = append(e.list[:i], e.list[i+1:]...)
			e.cronTab.Remove(entryID)
			return nil
		}
	}
	return fmt.Errorf("entry not found: %d", id)
}

// Start starts the cf-crontab server
func Start() {
	// Config
	viper.SetConfigName("config")
	viper.SetConfigType("yml")
	viper.SetEnvPrefix("cf-crontab")
	viper.AutomaticEnv()
	viper.AddConfigPath(".")
	_ = viper.ReadInConfig()

	// Cron
	table := &State {
		cronTab: cron.New(cron.WithSeconds()),
		list: make([]*crontab.Task, 0),
	}
	entries, err := crontab.LoadFromEnv()
	if err != nil {
		fmt.Printf("error loading config: %v\n", err)
		return
	}
	table.AddEntries(entries)
	table.StartCron()

	// Echo
	e := echo.New()
	e.GET("/entries", entriesGetHandler(table))
	e.POST("/entries", entriesPostHandler(table))
	e.DELETE("/entries/:entryID", entriesDeleteHandler(table))
	usePort := os.Getenv("PORT")
	if usePort == "" {
		usePort = "8080"
	}
	_ = e.Start(":" + usePort)
}
