package commands

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"
  "sort"

	"github.com/jdollar/backup/internal/box"
	"github.com/jdollar/backup/internal/config"
	"github.com/urfave/cli/v2"
)

const OUTPUT_DIRECTORY_FLAG = "outputDirectory"

type RequiredStringField struct {
  Value string
  Err string
}

type RequiredIntField struct {
  Value int64
  Err string
}

func validateConfigValues(conf config.Configuration) error {
  boxConf := conf.Box

  requiredStringFields := []RequiredStringField {
    {
      Value: boxConf.BackupFolderName,
      Err: "backup_folder_name",
    },
    {
      Value: boxConf.ClientID,
      Err: "client_id",
    },
    {
      Value: boxConf.ClientSecret,
      Err: "client_secret",
    },
    {
      Value: boxConf.SubjectType,
      Err: "subject_type",
    },
    {
      Value: boxConf.SubjectId,
      Err: "subject_id",
    },
  }

  requiredIntFields := []RequiredIntField{
    {
      Value: conf.BackupLimit,
      Err: "backup_limit",
    },
  }

  for _, valiationConf := range requiredStringFields {
    if valiationConf.Value == "" {
      return errors.New("Missing box " + valiationConf.Err)
    }
  }

  for _, valiationConf := range requiredIntFields {
    if valiationConf.Value == 0 {
      return errors.New("Missing " + valiationConf.Err)
    }
  }

  return nil
}

func exportToBox(conf config.Configuration, file *os.File) error {
  // Validate config file to ensure we have
  // the required values
  err := validateConfigValues(conf)
  if err != nil {
    return err
  }

  ctx := context.Background()

  boxConf := conf.Box
  copts := box.ClientOpts{
    SubjectType: boxConf.SubjectType,
    SubjectId: boxConf.SubjectId,
    ClientID: boxConf.ClientID,
    ClientSecret: boxConf.ClientSecret,
  }

  client := box.NewClient(ctx, copts)

  log.Println("Looking for backup folder: " + boxConf.BackupFolderName)
  searchResponse, err := client.SearchFolders(boxConf.BackupFolderName)
  if err != nil {
    return err
  }

  var folder box.Folder
  for _, v := range searchResponse.Entries {
    if v.Name == boxConf.BackupFolderName {
      log.Println("Found backup folder")
      folder = v
      break
    }
  }

  if folder == (box.Folder{}) {
    log.Println("No backup folder found. Creating " + boxConf.BackupFolderName)

    createFolderReq := box.CreateFolderRequest{
      Name: boxConf.BackupFolderName,
      Parent: box.Folder{
        Id: "0",
      },
    }
    createResponse, err := client.CreateBackupFolder(createFolderReq)
    if err != nil {
      return err
    }

    folder = box.Folder(createResponse)
  }

  log.Println("Uploading backup file to box")
  // Upload the new backup file
  err = client.Upload(folder, file)
  if err != nil {
    return err
  }
  log.Println("Finished backing up file to box")

  log.Println("Cleaning up old backups")
  // Grab all the files now in the folder
  listResp, err := client.ListItemsInFolder(
    folder,
    999,
    0,
  )
  if err != nil {
    return err
  }

  if int64(len(listResp.Entries)) > conf.BackupLimit {
    filesToRemove := listResp.Entries[conf.BackupLimit:]

    for _, fileToRemove := range filesToRemove {
      err := client.DeleteFile(fileToRemove)
      if err != nil {
        return err
      }
    }
  }
  log.Println("Finished cleaning old backups")

  return nil
}

type ByName []string

func (a ByName) Len() int           { return len(a) }
func (a ByName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByName) Less(i, j int) bool { return a[i] < a[j] }

func fileSystemCleanup(conf config.Configuration, outputPath string) error {
  filenames, err := filepath.Glob(outputPath + "/*.tar.gz")
  if err != nil {
    return err
  }

  numberToRemove := int64(len(filenames)) - conf.BackupLimit
  if numberToRemove <= 0 {
    log.Println("No files to remove for local backup")
    return nil
  }

  sort.Sort(ByName(filenames))

  filesToRemove := filenames[:numberToRemove]

  for _, filename := range filesToRemove {
    log.Println("Removing " + filename)
    err := os.Remove(filename)
    if err != nil {
      return  err
    }
  }

  return nil
}

func addToArchive(tw *tar.Writer, filename string, file io.Reader, info os.FileInfo) error {
  log.Println("Adding " + filename)
  header, err := tar.FileInfoHeader(info, info.Name())
  if err != nil {
    return err
  }

  header.Name = filename

  err = tw.WriteHeader(header)
  if err != nil {
    return err
  }

  _, err = io.Copy(tw, file)
  if err != nil {
    return err
  }

  return nil
}

func addFilesToArchive(tw *tar.Writer, files []string) error {
  for _, filenameOrGlob := range files {
    filenames, err := filepath.Glob(filenameOrGlob)
    if err != nil {
      return err
    }

    if len(filenames) <= 0 {
      return errors.New("No files found for backup")
    }

    for _, filename := range filenames {
      file, err := os.Open(filename)
      if err != nil {
        return err
      }
      defer file.Close()

      info, err := file.Stat()
      if err != nil {
        return err
      }

      if !info.IsDir() {
        err = addToArchive(tw, filename, file, info)
        if err != nil {
          return err
        }
      } else {
        dirFiles, err:= ioutil.ReadDir(filename)
        if err != nil {
          return err
        }

        var dirFileNames []string
        for _, dirFile := range dirFiles {
          dirFileName := filepath.Join(
            filepath.Dir(filename),
            info.Name(),
            dirFile.Name(),
          )

          dirFileNames = append(dirFileNames, dirFileName)
        }

        if len(dirFileNames) > 0 {
          err = addFilesToArchive(tw, dirFileNames)
          if err != nil {
            return err
          }
        }
      }
    }
  }

  return nil
}

func createArchive(files []string, buf io.Writer) error {
  gw := gzip.NewWriter(buf)
  defer gw.Close()
  tw := tar.NewWriter(gw)
  defer tw.Close()

  err := addFilesToArchive(tw, files)
  if err != nil {
    return err
  }

  return nil
}

func moveFile(oldFileName string, newFileName string) error {
  oldFile, err := os.Open(oldFileName)
  if err != nil {
    return err
  }

  newFile, err := os.Create(newFileName)
  if err != nil {
    return err
  }
  defer newFile.Close()

  _, err = io.Copy(newFile, oldFile)
  oldFile.Close()
  if err != nil {
    return err
  }

  err = os.Remove(oldFileName)
  if err != nil {
    return err
  }

  return nil
}

func boxCommandAction(conf config.Configuration, c *cli.Context) error {
  outputDirectory := c.String(OUTPUT_DIRECTORY_FLAG)
  err := os.MkdirAll(outputDirectory, os.ModePerm)
  if err != nil {
    return err
  }

  currentTimeUnix := time.Now().UTC().UnixMilli()

  outputFileName := strconv.FormatInt(currentTimeUnix, 10) + ".tar.gz"

  // create output file
  outputPath := filepath.Join(
    c.String(OUTPUT_DIRECTORY_FLAG),
    outputFileName,
  )

  tmpOut, err := ioutil.TempFile("", outputFileName)
  if err != nil {
    log.Fatal("Error backing up files:", err)
  }

  filenames := c.Args().Slice()
  err = createArchive(filenames, tmpOut)
  if err != nil {
    log.Fatal("Error backing up files:", err)
  }

  err = tmpOut.Close()
  if err != nil {
    return err
  }

  err = moveFile(tmpOut.Name(), outputPath)
  if err != nil {
    return err
  }

  outputFile, err := os.Open(outputPath)
  if err != nil {
    log.Fatal("Error exporting file:", err)
  }
  defer outputFile.Close()

  err = fileSystemCleanup(conf, c.String(OUTPUT_DIRECTORY_FLAG))
  if err != nil {
    return err
  }


  log.Println(outputPath)
  err = exportToBox(conf, outputFile)
  if err != nil {
    log.Fatal("Error exporting file:", err)
  }

  return nil
}

func NewBackupCommand(conf config.Configuration) *cli.Command {
  commandAction := func(c *cli.Context) error {
    return boxCommandAction(conf, c)
  }

  return &cli.Command{
    Name: "box",
    Usage: "Command to backup to dropbox",
    Flags: []cli.Flag{
      &cli.StringFlag{
        Name: OUTPUT_DIRECTORY_FLAG,
        Aliases: []string{"o"},
        Usage: "Path to where we will shove output",
        Required: true,
      },
    },
    Action: commandAction,
  }
}

