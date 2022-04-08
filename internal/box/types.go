package box

type ClientError struct {
  Type string `json:"type"`
  Status int64 `json:"status"`
  Code string `json:"code"`
  ContextInfo interface{} `json:"context_info"`
  HelpUrl string `json:"help_url"`
  Message string `json:"message"`
  RequestId string `json:"request_id"`
}

type Folder struct {
  Id string `json:"id"`
  Type string `json:"type"`
  Name string `json:"name"`
}

type File struct {
  Id string `json:"id"`
  Type string `json:"type"`
  Name string `json:"name"`
}

type SearchResponse struct {
  TotalCount int64 `json:"total_count"`
  Limit int64 `json:"limit"`
  Offset int64 `json:"offset"`
  Entries []Folder `json:"entries"`
}

type ListItemsInFolderRequest struct {
  Limit int64
  Offset int64
}

type ListItemsInFolderResponse struct {
  TotalCount int64 `json:"total_count"`
  Limit int64 `json:"limit"`
  Offset int64 `json:"offset"`
  Entries []File `json:"entries"`
}

type CreateFolderRequest struct {
  Name string `json:"name"`
  Parent Folder `json:"parent"`
}

type CreateFolderResponse Folder

type CreateUploadSessionRequest struct {
  FileName string `json:"file_name"`
  FileSize int64 `json:"file_size"`
  FolderId string `json:"folder_id"`
}

type SessionEndpoints struct {
  Abort string `json:"abort"`
  Commit string `json:"commit"`
  ListParts string `json:"list_parts"`
  LogEvent string `json:"log_event"`
  Status string `json:"status"`
  UploadPart string `json:"upload_part"`
}

type CreateUploadSessionResponse struct {
  Id string `json:"id"`
  Type string `json:"type"`
  NumPartsProcessed int32 `json:"num_parts_processed"`
  PartSize int64 `json:"part_size"`
  SessionEndpoints SessionEndpoints `json:"session_endpoints"`
  SessionExpiresAt string `json:"session_expires_at"`
  TotalParts int32 `json:"total_parts"`
}

type UploadPart struct {
  Offset int64 `json:"offset"`
  PartId string `json:"part_id"`
  sha1 string `json:"sha1"`
  size int64 `json:"size"`
}

type CommitUploadSessionRequest struct {
  Parts []UploadPart `json:"parts"`
}

type CommitUploadSessionResponse struct {
  Entries []File `json:"entries"`
  TotalCount int64 `json:"total_count"`
}

type UploadPartResponse struct {
  Part UploadPart `json:"part"`
}

type UploadResponse struct {
  Entries []File `json:"entries"`
  TotalCount int64 `json:"total_count"`
}
