package box

import (
	"bytes"
  "log"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
  "mime/multipart"
  "time"
  "strconv"
  "sort"

  "github.com/jdollar/backup/internal/files"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

const TWENTY_MB = 20*1024

type ClientOpts struct {
  SubjectType string
  SubjectId string
  ClientID string
  ClientSecret string
}

type Client struct {
  httpClient *http.Client
}

func NewClient(ctx context.Context, copts ClientOpts) Client {
  tokenParams := url.Values{}
  tokenParams.Set("box_subject_type", copts.SubjectType)
  tokenParams.Set("box_subject_id", copts.SubjectId)

  conf := clientcredentials.Config{
    ClientID: copts.ClientID,
    ClientSecret: copts.ClientSecret,
    Scopes: []string{
      "root_readwrite",
    },
    EndpointParams: tokenParams,
    TokenURL: "https://api.box.com/oauth2/token",
    AuthStyle: oauth2.AuthStyleInParams,
  }

  client := Client{}
  client.httpClient = conf.Client(ctx)

  return client
}

func (c *Client) handleResponse(resp *http.Response, result interface{}) error {
  if resp.StatusCode < 200 || resp.StatusCode >= 300 {
    var errResp ClientError
    err := json.NewDecoder(resp.Body).Decode(&errResp)
    if err != nil {
      return err
    }

    if errResp.Message != "" {
      return errors.New(errResp.Message)
    }
  }

  if resp.StatusCode != 204 {
    err := json.NewDecoder(resp.Body).Decode(&result)
    if err != nil {
      return err
    }
  }

  return nil
}

func (c *Client) makeRequest(req *http.Request, resp interface{}) error {
  rawResp, err := c.httpClient.Do(req)
  if err != nil {
    return err
  }

  return c.handleResponse(rawResp, &resp)
}

func (c *Client) SearchFolders(name string) (SearchResponse, error) {
  var searchResponse SearchResponse

  req, err := http.NewRequest(
    http.MethodGet,
    "https://api.box.com/2.0/search",
    nil,
  )
  if err != nil {
    return searchResponse, err
  }

  q := req.URL.Query()
  q.Add("query", name)
  req.URL.RawQuery = q.Encode()

  err = c.makeRequest(req, &searchResponse)
  return searchResponse, err
}

func (c *Client) ListItemsInFolder(folder Folder, limit int64, offset int64) (ListItemsInFolderResponse, error) {
  var resp ListItemsInFolderResponse

  req, err := http.NewRequest(
    http.MethodGet,
    fmt.Sprintf("https://api.box.com/2.0/folders/%s/items", folder.Id),
    nil,
  )
  if err != nil {
    return resp, err
  }

  q := req.URL.Query()
  q.Add("limit", strconv.FormatInt(limit, 10))
  q.Add("offset", strconv.FormatInt(offset, 10))
  q.Add("sort", "name")
  q.Add("direction", "DESC")
  req.URL.RawQuery = q.Encode()

  err = c.makeRequest(req, &resp)
  return resp, err
}

func (c *Client) DeleteFile(file File) error {
  req, err := http.NewRequest(
    http.MethodDelete,
    fmt.Sprintf("https://api.box.com/2.0/files/%s", file.Id),
    nil,
  )
  if err != nil {
    return err
  }

  err = c.makeRequest(req, nil)
  return err
}

func (c *Client) CreateBackupFolder(reqBody CreateFolderRequest) (CreateFolderResponse, error) {
  var resp CreateFolderResponse

  jsonBody, err := json.Marshal(reqBody)
  if err != nil {
    return resp, err
  }

  req, err := http.NewRequest(
    http.MethodPost,
    "https://api.box.com/2.0/folders",
    bytes.NewBuffer(jsonBody),
  )
  if err != nil {
    return resp, err
  }

  err = c.makeRequest(req, &resp)
  return resp, err
}

func (c *Client) CreateUploadSession(req CreateUploadSessionRequest) (CreateUploadSessionResponse, error) {
  var resp CreateUploadSessionResponse

  jsonBody, err := json.Marshal(req)
  if err != nil {
    return resp, err
  }

  httpReq, err := http.NewRequest(
    http.MethodPost,
    "https://upload.box.com/api/2.0/files/upload_sessions",
    bytes.NewBuffer(jsonBody),
  )
  if err != nil {
    return resp, err
  }

  err = c.makeRequest(httpReq, &resp)
  return resp, err
}

func (c *Client) GetUploadSession(sessionId string) (GetUploadSessionResponse, error) {
  var resp GetUploadSessionResponse

  httpReq, err := http.NewRequest(
    http.MethodGet,
    fmt.Sprintf("https://upload.box.com/api/2.0/files/upload_sessions/%s", sessionId),
    nil,
  )
  if err != nil {
    return resp, err
  }

  err = c.makeRequest(httpReq, &resp)
  return resp, err
}

type ByOffset []UploadPart

func (a ByOffset) Len() int           { return len(a) }
func (a ByOffset) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByOffset) Less(i, j int) bool { return a[i].Offset < a[j].Offset }

func (c *Client) CommitUploadSession(sessionId string, parts []UploadPart, digest string) (CommitUploadSessionResponse, error) {
  var resp CommitUploadSessionResponse

  sort.Sort(ByOffset(parts))

  req := CommitUploadSessionRequest{
    Parts: parts,
  }

  jsonBody, err := json.Marshal(req)
  if err != nil {
    return resp, err
  }

  httpReq, err := http.NewRequest(
    http.MethodPost,
    fmt.Sprintf("https://upload.box.com/api/2.0/files/upload_sessions/%s/commit", sessionId),
    bytes.NewBuffer(jsonBody),
  )
  if err != nil {
    return resp, err
  }

  httpReq.Header.Set("digest", "sha=" + digest)

  err = c.makeRequest(httpReq, &resp)
  return resp, err
}

type UploadAttributes struct {
  ContentCreatedAt string `json:"content_created_at"`
  ContentModifiedAt string `json:"content_modified_at"`
  Name string `json:"name"`
  Parent Folder `json:"parent"`
}

func (c *Client) Upload(folder Folder, file *os.File) error {
  info, err := file.Stat()
  if err != nil {
    return err
  }

  if info.Size() >= TWENTY_MB {
    return c.chunkedUpload(folder, file)
  }

  return c.singleUpload(folder, file)
}

func (c *Client) singleUpload(folder Folder, file *os.File) error {
  log.Println("Doing single upload")

  body := &bytes.Buffer{}
  w := multipart.NewWriter(body)

  info, err := file.Stat()
  if err != nil {
    return err
  }

  // add fields
  currentDate := time.Now().UTC().Format(time.RFC3339)
  jsonBody, err := json.Marshal(UploadAttributes{
    ContentCreatedAt: currentDate,
    ContentModifiedAt: currentDate,
    Name: info.Name(),
    Parent: folder,
  })

  fw, err := w.CreateFormField("attributes")
  if err != nil {
    return err
  }
  _, err = io.Copy(fw, bytes.NewBuffer(jsonBody))
  if err != nil {
    return err
  }

  fw, err = w.CreateFormFile("file", info.Name())
  if err != nil {
    return err
  }

  _, err = io.Copy(fw, file)
  if err != nil {
    return err
  }

  err = w.Close()
  if err != nil {
    return err
  }

  httpReq, err := http.NewRequest(
    http.MethodPost,
    "https://upload.box.com/api/2.0/files/content",
    body,
  )
  if err != nil {
    return err
  }

  httpReq.Header.Set("Content-Type", w.FormDataContentType())

  var resp UploadResponse
  err = c.makeRequest(httpReq, &resp)
  return err
}

func (c *Client) chunkedUpload(folder Folder, file *os.File) error {
  log.Println("Doing chunk upload")

  info, err := file.Stat()
  if err != nil {
    return err
  }

  createSessionReq := CreateUploadSessionRequest{
    FileName: info.Name(),
    FileSize: info.Size(),
    FolderId: folder.Id,
  }

  log.Println("Creating upload session")
  createUploadSessionResponse, err := c.CreateUploadSession(createSessionReq)
  if err != nil {
    return err
  }
  log.Println("Created upload session")

  parts, err := files.ChunkFile(file, createUploadSessionResponse.PartSize)
  if err != nil {
    return err
  }

  var uploadedParts []UploadPart
  uploadChan := make(chan error)
  for _, part := range parts {
    go func(part files.FilePart) {
      httpReq, err := http.NewRequest(
        http.MethodPut,
        fmt.Sprintf("https://upload.box.com/api/2.0/files/upload_sessions/%s", createUploadSessionResponse.Id),
        bytes.NewBuffer(part.Data),
      )

      base64encodedDigest := base64.StdEncoding.EncodeToString(part.Digest)

      httpReq.Header.Set("content-type", "application/octet-stream")
      httpReq.Header.Set(
        "content-range",
        fmt.Sprintf("bytes %d-%d/%d", part.Begin, part.End, info.Size()),
      )
      httpReq.Header.Set(
        "digest",
        fmt.Sprintf("sha=%s", base64encodedDigest),
      )

      log.Println("Uploading part " + base64encodedDigest)
      rawUploadResp, err := c.httpClient.Do(httpReq)
      if err != nil {
        uploadChan <- err
      }
      log.Println("Finished uploading part " + base64encodedDigest)

      var uploadPartResponse UploadPartResponse
      err = c.handleResponse(rawUploadResp, &uploadPartResponse)
      if err != nil {
        uploadChan <- err
      }

      uploadedParts = append(uploadedParts, uploadPartResponse.Part)

      uploadChan <- nil
    }(part)
  }

  for i := 0; i < len(parts); i++ {
    err = <- uploadChan
    if err != nil {
      return err
    }
  }

  log.Println("Checking session state")

  for {
    getUploadSessionResponse, err := c.GetUploadSession(createUploadSessionResponse.Id)
    if err != nil {
      return err
    }

    processed := getUploadSessionResponse.NumPartsProcessed
    total := getUploadSessionResponse.TotalParts

    if processed == total {
      break
    }

    log.Println(strconv.FormatInt(int64(processed), 10) + " processed out of " + strconv.FormatInt(int64(total), 10))
    time.Sleep(1 * time.Second)
  }

  log.Println("Session Ready!")

  log.Println("Committing session")
  fileHash := sha1.New()
  digestFile, err := os.Open(file.Name())
  if err != nil {
    return err
  }
  defer digestFile.Close()

  if _, err := io.Copy(fileHash, digestFile); err != nil {
    return err
  }
  digest := base64.StdEncoding.EncodeToString(fileHash.Sum(nil))

  _, err = c.CommitUploadSession(createUploadSessionResponse.Id, uploadedParts, digest)
  if err != nil {
    return err
  }
  log.Println("Commited session")

  return nil
}
