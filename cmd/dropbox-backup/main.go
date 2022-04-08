package main

import (
	"log"
	"os"

	"github.com/urfave/cli/v2"
	"github.com/jdollar/dropbox-backup/internal/commands"
	"github.com/jdollar/dropbox-backup/internal/config"
)

func main() {
  conf, err := config.NewConfiguration()
  if err != nil {
    log.Fatal(err)
  }

  app := &cli.App{
    Name: "backup",
    Usage: "Cli tool to backup files to dropbox",
    Commands: []*cli.Command{
      commands.NewBackupCommand(conf),
    },
  }

  err = app.Run(os.Args)
  if err != nil {
    log.Fatal(err)
  }
}
