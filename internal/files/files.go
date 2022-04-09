package files

import (
  "bufio"
  "crypto/sha1"
  "io"
  "os"
)

type FilePart struct {
  Begin int64
  End int64
  Data []byte
  Digest []byte
}

func ChunkFile (file *os.File, partSize int64) ([]FilePart, error) {
  nBytes := int64(0)
  r := bufio.NewReader(file)
  buf := make([]byte, 0, partSize)

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

      return parts, err
    }

    begin := nBytes
    end := begin + int64(len(buf) - 1)
    h := sha1.New()
    h.Write(buf)
    d := h.Sum(nil)

    data := make([]byte, len(buf))
    copy(data, buf)

    part := FilePart{
      Begin: begin,
      End: end,
      Data: data,
      Digest: d,
    }
    parts = append(parts, part)

    nBytes += int64(len(buf))
    if err != nil && err != io.EOF {
      return parts, err
    }
  }

  return parts, nil
}
