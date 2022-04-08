package box

import (
	"bufio"
	"bytes"
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

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
)

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


func (c *Client) SearchFolders(name string) (SearchResponse, error) {
  var searchResponse SearchResponse

  req, err := http.NewRequest(
    "GET",
    "https://api.box.com/2.0/search",
    nil,
  )
  if err != nil {
    return searchResponse, err
  }

  q := req.URL.Query()
  q.Add("query", "minecraftBackups")
  req.URL.RawQuery = q.Encode()

  resp, err := c.httpClient.Do(req)
  if err != nil {
    return searchResponse, err
  }
  err = c.handleResponse(resp, &searchResponse)
  if err != nil {
    return searchResponse, err
  }

  return searchResponse, nil
}

func (c *Client) ListItemsInFolder(folder Folder, limit int64, offset int64) (ListItemsInFolderResponse, error) {
  var resp ListItemsInFolderResponse

  req, err := http.NewRequest(
    "GET",
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

  rawResp, err := c.httpClient.Do(req)
  if err != nil {
    return resp, err
  }
  err = c.handleResponse(rawResp, &resp)
  if err != nil {
    return resp, err
  }

  return resp, nil
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

  rawResp, err := c.httpClient.Do(req)
  if err != nil {
    return err
  }
  err = c.handleResponse(rawResp, nil)
  if err != nil {
    return err
  }

  return nil
}

func (c *Client) CreateBackupFolder(reqBody CreateFolderRequest) (CreateFolderResponse, error) {
  var resp CreateFolderResponse

  jsonBody, err := json.Marshal(reqBody)
  if err != nil {
    return resp, err
  }

  req, err := http.NewRequest(
    "POST",
    "https://api.box.com/2.0/folders",
    bytes.NewBuffer(jsonBody),
  )
  if err != nil {
    return resp, err
  }

  rawResp, err := c.httpClient.Do(req)
  if err != nil {
    return resp, err
  }

  err = c.handleResponse(rawResp, &resp)
  if err != nil {
    return resp, err
  }

  return resp, nil
}

func (c *Client) CreateUploadSession(req CreateUploadSessionRequest) (CreateUploadSessionResponse, error) {
  var createUploadSessionResponse CreateUploadSessionResponse

  jsonBody, err := json.Marshal(req)
  if err != nil {
    return createUploadSessionResponse, err
  }

  httpReq, err := http.NewRequest(
    "POST",
    "https://upload.box.com/api/2.0/files/upload_sessions",
    bytes.NewBuffer(jsonBody),
  )
  if err != nil {
    return createUploadSessionResponse, err
  }

  rawCreateSessionResp, err := c.httpClient.Do(httpReq)
  if err != nil {
    return createUploadSessionResponse, err
  }

  err = c.handleResponse(rawCreateSessionResp, &createUploadSessionResponse)
  if err != nil {
    return createUploadSessionResponse, err
  }

  return createUploadSessionResponse, nil
}

func (c *Client) CommitUploadSession(sessionId string, parts []UploadPart) (CommitUploadSessionResponse, error) {
  var commitUploadSessionResponse CommitUploadSessionResponse

  req := CommitUploadSessionRequest{
    Parts: parts,
  }

  jsonBody, err := json.Marshal(req)
  if err != nil {
    return commitUploadSessionResponse, err
  }

  httpReq, err := http.NewRequest(
    "POST",
    fmt.Sprintf("https://upload.box.com/api/2.0/files/upload_sessions/%s/commit", sessionId),
    bytes.NewBuffer(jsonBody),
  )
  if err != nil {
    return commitUploadSessionResponse, err
  }

  rawCommitSessionResp, err := c.httpClient.Do(httpReq)
  if err != nil {
    return commitUploadSessionResponse, err
  }

  err = c.handleResponse(rawCommitSessionResp, &commitUploadSessionResponse)
  if err != nil {
    return commitUploadSessionResponse, err
  }

  return commitUploadSessionResponse, nil
}

type FilePart struct {
  Begin int64
  End int64
  Data []byte
  Digest string
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

  if info.Size() >= 20*1024 {
    return c.chunkedUpload(folder, file)
  }

  return c.singleUpload(folder, file)
}

func (c *Client) singleUpload(folder Folder, file *os.File) error {
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

  rawUploadResp, err := c.httpClient.Do(httpReq)
  if err != nil {
    return err
  }

  var uploadResponse UploadResponse
  err = c.handleResponse(rawUploadResp, &uploadResponse)
  if err != nil {
    return err
  }

  return nil
}

func (c *Client) chunkedUpload(folder Folder, file *os.File) error {
  nBytes := int64(0)
  r := bufio.NewReader(file)
  buf := make([]byte, 0, 4*1024)

  var parts []FilePart
  for {
    n, err := r.Read(buf[:cap(buf)])
    buf = buf[:n]
    if n == 0 {
      if err == nil {
        continue
      }

      if err == io.EOF {
        break
      }

      return err
    }

    begin := nBytes
    end := begin + int64(len(buf))
    digest := sha1.New()
    digest.Write(buf)

    part := FilePart{
      Begin: begin,
      End: end,
      Data: buf,
      Digest: base64.StdEncoding.EncodeToString(digest.Sum(nil)),
    }
    parts = append(parts, part)

    nBytes += int64(len(buf))
    if err != nil && err != io.EOF {
      return err
    }
  }

  fmt.Println(parts)

  info, err := file.Stat()
  if err != nil {
    return err
  }

  createSessionReq := CreateUploadSessionRequest{
    FileName: info.Name(),
    FileSize: info.Size(),
    FolderId: folder.Id,
  }

  createUploadSessionResponse, err := c.CreateUploadSession(createSessionReq)
  if err != nil {
    return err
  }

  var uploadedParts []UploadPart
  for _, part := range parts {
    httpReq, err := http.NewRequest(
      "POST",
      fmt.Sprintf("https://upload.box.com/api/2.0/files/upload_sessions/%s", createUploadSessionResponse.Id),
      bytes.NewBuffer(part.Data),
    )

    httpReq.Header.Set("Content-Type", "application/octet-stream")
    httpReq.Header.Set("content-range", fmt.Sprintf("%d - %d/%d", part.Begin, part.End, part.End - part.Begin))
    httpReq.Header.Set("digest", fmt.Sprintf("sha1=%s", part.Digest))

    rawUploadResp, err := c.httpClient.Do(httpReq)
    if err != nil {
      return err
    }

    var uploadPartResponse UploadPartResponse
    err = c.handleResponse(rawUploadResp, &uploadPartResponse)
    if err != nil {
      return err
    }

    uploadedParts = append(uploadedParts, uploadPartResponse.Part)
  }

  _, err = c.CommitUploadSession(createUploadSessionResponse.Id, uploadedParts)
  if err != nil {
    return err
  }

  return nil
}
