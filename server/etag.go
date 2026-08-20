package server

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"github.com/felixge/httpsnoop"
)

// DefaultETagMaxBodyBytes is the default response size limit for automatic
// ETag generation. Larger responses are streamed without a generated ETag.
const DefaultETagMaxBodyBytes = 1 << 20

type etagResponseWriter struct {
	w            http.ResponseWriter
	maxBodyBytes int
	status       int
	wroteHeader  bool
	passthrough  bool
	body         bytes.Buffer
}

func withETag(next http.Handler, maxBodyBytes int) http.Handler {
	if maxBodyBytes <= 0 {
		maxBodyBytes = DefaultETagMaxBodyBytes
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ew := &etagResponseWriter{w: w, maxBodyBytes: maxBodyBytes}
		wrapped := httpsnoop.Wrap(w, httpsnoop.Hooks{
			WriteHeader: func(next httpsnoop.WriteHeaderFunc) httpsnoop.WriteHeaderFunc {
				return ew.writeHeader(next)
			},
			Write: func(next httpsnoop.WriteFunc) httpsnoop.WriteFunc {
				return ew.write(next)
			},
			Flush: func(next httpsnoop.FlushFunc) httpsnoop.FlushFunc {
				return func() {
					_ = ew.startPassthrough(true)
					next()
				}
			},
			FlushError: func(next httpsnoop.FlushErrorFunc) httpsnoop.FlushErrorFunc {
				return func() error {
					return errors.Join(ew.startPassthrough(true), next())
				}
			},
			Hijack: func(next httpsnoop.HijackFunc) httpsnoop.HijackFunc {
				return func() (net.Conn, *bufio.ReadWriter, error) {
					if err := ew.startPassthrough(false); err != nil {
						return nil, nil, err
					}
					return next()
				}
			},
			ReadFrom: func(_ httpsnoop.ReadFromFunc) httpsnoop.ReadFromFunc {
				return func(src io.Reader) (int64, error) {
					return io.Copy(etagWriterFunc(ew.writeBody), src)
				}
			},
			EnableFullDuplex: func(next httpsnoop.EnableFullDuplexFunc) httpsnoop.EnableFullDuplexFunc {
				return func() error {
					if err := ew.startPassthrough(false); err != nil {
						return err
					}
					return next()
				}
			},
		})

		next.ServeHTTP(wrapped, r)
		if err := ew.finish(r); err != nil {
			slog.Error("ETag response", "err", err)
		}
	})
}

func (ew *etagResponseWriter) writeHeader(next httpsnoop.WriteHeaderFunc) httpsnoop.WriteHeaderFunc {
	return func(code int) {
		if code >= 100 && code < 200 {
			next(code)
			return
		}
		if ew.passthrough {
			next(code)
			return
		}
		if ew.wroteHeader {
			return
		}
		ew.status = code
		ew.wroteHeader = true
	}
}

func (ew *etagResponseWriter) write(next httpsnoop.WriteFunc) httpsnoop.WriteFunc {
	return func(body []byte) (int, error) {
		if ew.passthrough {
			return next(body)
		}
		return ew.writeBody(body)
	}
}

func (ew *etagResponseWriter) writeBody(body []byte) (int, error) {
	if ew.passthrough {
		return ew.w.Write(body)
	}
	if ew.status == 0 {
		ew.status = http.StatusOK
	}
	if ew.body.Len()+len(body) <= ew.maxBodyBytes {
		return ew.body.Write(body)
	}
	if err := ew.startPassthrough(false); err != nil {
		return 0, err
	}
	return ew.w.Write(body)
}

func (ew *etagResponseWriter) startPassthrough(commitEmpty bool) error {
	if ew.passthrough {
		return nil
	}
	ew.passthrough = true

	if ew.wroteHeader {
		ew.w.WriteHeader(ew.status)
	} else if ew.body.Len() == 0 && commitEmpty {
		ew.w.WriteHeader(http.StatusOK)
	}
	if ew.body.Len() == 0 {
		return nil
	}
	_, err := ew.w.Write(ew.body.Bytes())
	ew.body.Reset()
	return err
}

func (ew *etagResponseWriter) finish(r *http.Request) error {
	if ew.passthrough {
		return nil
	}
	if ew.status == 0 {
		// Match net/http: a handler which writes nothing produces an implicit 200.
		return nil
	}

	body := ew.body.Bytes()
	if ew.status == http.StatusOK && len(body) != 0 && ew.w.Header().Get("ETag") == "" {
		if ew.w.Header().Get("Content-Type") == "" {
			ew.w.Header().Set("Content-Type", http.DetectContentType(body))
		}
		sum := sha256.Sum256(body)
		tag := fmt.Sprintf(`"%x"`, sum)
		ew.w.Header().Set("ETag", tag)
		if (r.Method == http.MethodGet || r.Method == http.MethodHead) && etagMatches(r.Header.Get("If-None-Match"), tag) {
			ew.w.Header().Del("Content-Length")
			ew.w.WriteHeader(http.StatusNotModified)
			return nil
		}
	}

	if ew.wroteHeader {
		ew.w.WriteHeader(ew.status)
	}
	if len(body) == 0 {
		return nil
	}
	_, err := ew.w.Write(body)
	return err
}

func etagMatches(header, tag string) bool {
	for value := range strings.SplitSeq(header, ",") {
		value = strings.TrimSpace(value)
		if value == "*" || strings.TrimPrefix(value, "W/") == tag {
			return true
		}
	}
	return false
}

type etagWriterFunc func([]byte) (int, error)

func (fn etagWriterFunc) Write(body []byte) (int, error) {
	return fn(body)
}
