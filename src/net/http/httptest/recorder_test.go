// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package httptest

import (
	"fmt"
	"io"
	"net/http"
	"testing"
)

func TestRecorder(t *testing.T) {
	type checkFunc func(*ResponseRecorder) error
	check := func fns { return fns }

	hasStatus := func wantCode {
		return func rec {
			if rec.Code != wantCode {
				return fmt.Errorf("Status = %d; want %d", rec.Code, wantCode)
			}
			return nil
		}
	}
	hasResultStatus := func want {
		return func rec {
			if rec.Result().Status != want {
				return fmt.Errorf("Result().Status = %q; want %q", rec.Result().Status, want)
			}
			return nil
		}
	}
	hasResultStatusCode := func wantCode {
		return func rec {
			if rec.Result().StatusCode != wantCode {
				return fmt.Errorf("Result().StatusCode = %d; want %d", rec.Result().StatusCode, wantCode)
			}
			return nil
		}
	}
	hasContents := func want {
		return func rec {
			if rec.Body.String() != want {
				return fmt.Errorf("wrote = %q; want %q", rec.Body.String(), want)
			}
			return nil
		}
	}
	hasFlush := func want {
		return func rec {
			if rec.Flushed != want {
				return fmt.Errorf("Flushed = %v; want %v", rec.Flushed, want)
			}
			return nil
		}
	}
	hasOldHeader := func key, want {
		return func rec {
			if got := rec.HeaderMap.Get(key); got != want {
				return fmt.Errorf("HeaderMap header %s = %q; want %q", key, got, want)
			}
			return nil
		}
	}
	hasHeader := func key, want {
		return func rec {
			if got := rec.Result().Header.Get(key); got != want {
				return fmt.Errorf("final header %s = %q; want %q", key, got, want)
			}
			return nil
		}
	}
	hasNotHeaders := func keys {
		return func rec {
			for _, k := range keys {
				v, ok := rec.Result().Header[http.CanonicalHeaderKey(k)]
				if ok {
					return fmt.Errorf("unexpected header %s with value %q", k, v)
				}
			}
			return nil
		}
	}
	hasTrailer := func key, want {
		return func rec {
			if got := rec.Result().Trailer.Get(key); got != want {
				return fmt.Errorf("trailer %s = %q; want %q", key, got, want)
			}
			return nil
		}
	}
	hasNotTrailers := func keys {
		return func rec {
			trailers := rec.Result().Trailer
			for _, k := range keys {
				_, ok := trailers[http.CanonicalHeaderKey(k)]
				if ok {
					return fmt.Errorf("unexpected trailer %s", k)
				}
			}
			return nil
		}
	}
	hasContentLength := func length {
		return func rec {
			if got := rec.Result().ContentLength; got != length {
				return fmt.Errorf("ContentLength = %d; want %d", got, length)
			}
			return nil
		}
	}

	tests := []struct {
		name   string
		h      func(w http.ResponseWriter, r *http.Request)
		checks []checkFunc
	}{
		{
			"200 default",
			func w, r {},
			check(hasStatus(200), hasContents("")),
		},
		{
			"first code only",
			func w, r {
				w.WriteHeader(201)
				w.WriteHeader(202)
				w.Write([]byte("hi"))
			},
			check(hasStatus(201), hasContents("hi")),
		},
		{
			"write sends 200",
			func w, r {
				w.Write([]byte("hi first"))
				w.WriteHeader(201)
				w.WriteHeader(202)
			},
			check(hasStatus(200), hasContents("hi first"), hasFlush(false)),
		},
		{
			"write string",
			func w, r {
				io.WriteString(w, "hi first")
			},
			check(
				hasStatus(200),
				hasContents("hi first"),
				hasFlush(false),
				hasHeader("Content-Type", "text/plain; charset=utf-8"),
			),
		},
		{
			"flush",
			func w, r {
				w.(http.Flusher).Flush() // also sends a 200
				w.WriteHeader(201)
			},
			check(hasStatus(200), hasFlush(true), hasContentLength(-1)),
		},
		{
			"Content-Type detection",
			func w, r {
				io.WriteString(w, "<html>")
			},
			check(hasHeader("Content-Type", "text/html; charset=utf-8")),
		},
		{
			"no Content-Type detection with Transfer-Encoding",
			func w, r {
				w.Header().Set("Transfer-Encoding", "some encoding")
				io.WriteString(w, "<html>")
			},
			check(hasHeader("Content-Type", "")), // no header
		},
		{
			"no Content-Type detection if set explicitly",
			func w, r {
				w.Header().Set("Content-Type", "some/type")
				io.WriteString(w, "<html>")
			},
			check(hasHeader("Content-Type", "some/type")),
		},
		{
			"Content-Type detection doesn't crash if HeaderMap is nil",
			func w, r {
				// Act as if the user wrote new(httptest.ResponseRecorder)
				// rather than using NewRecorder (which initializes
				// HeaderMap)
				w.(*ResponseRecorder).HeaderMap = nil
				io.WriteString(w, "<html>")
			},
			check(hasHeader("Content-Type", "text/html; charset=utf-8")),
		},
		{
			"Header is not changed after write",
			func w, r {
				hdr := w.Header()
				hdr.Set("Key", "correct")
				w.WriteHeader(200)
				hdr.Set("Key", "incorrect")
			},
			check(hasHeader("Key", "correct")),
		},
		{
			"Trailer headers are correctly recorded",
			func w, r {
				w.Header().Set("Non-Trailer", "correct")
				w.Header().Set("Trailer", "Trailer-A")
				w.Header().Add("Trailer", "Trailer-B")
				w.Header().Add("Trailer", "Trailer-C")
				io.WriteString(w, "<html>")
				w.Header().Set("Non-Trailer", "incorrect")
				w.Header().Set("Trailer-A", "valuea")
				w.Header().Set("Trailer-C", "valuec")
				w.Header().Set("Trailer-NotDeclared", "should be omitted")
				w.Header().Set("Trailer:Trailer-D", "with prefix")
			},
			check(
				hasStatus(200),
				hasHeader("Content-Type", "text/html; charset=utf-8"),
				hasHeader("Non-Trailer", "correct"),
				hasNotHeaders("Trailer-A", "Trailer-B", "Trailer-C", "Trailer-NotDeclared"),
				hasTrailer("Trailer-A", "valuea"),
				hasTrailer("Trailer-C", "valuec"),
				hasNotTrailers("Non-Trailer", "Trailer-B", "Trailer-NotDeclared"),
				hasTrailer("Trailer-D", "with prefix"),
			),
		},
		{
			"Header set without any write", // Issue 15560
			func w, r {
				w.Header().Set("X-Foo", "1")

				// Simulate somebody using
				// new(ResponseRecorder) instead of
				// using the constructor which sets
				// this to 200
				w.(*ResponseRecorder).Code = 0
			},
			check(
				hasOldHeader("X-Foo", "1"),
				hasStatus(0),
				hasHeader("X-Foo", "1"),
				hasResultStatus("200 OK"),
				hasResultStatusCode(200),
			),
		},
		{
			"HeaderMap vs FinalHeaders", // more for Issue 15560
			func w, r {
				h := w.Header()
				h.Set("X-Foo", "1")
				w.Write([]byte("hi"))
				h.Set("X-Foo", "2")
				h.Set("X-Bar", "2")
			},
			check(
				hasOldHeader("X-Foo", "2"),
				hasOldHeader("X-Bar", "2"),
				hasHeader("X-Foo", "1"),
				hasNotHeaders("X-Bar"),
			),
		},
		{
			"setting Content-Length header",
			func w, r {
				body := "Some body"
				contentLength := fmt.Sprintf("%d", len(body))
				w.Header().Set("Content-Length", contentLength)
				io.WriteString(w, body)
			},
			check(hasStatus(200), hasContents("Some body"), hasContentLength(9)),
		},
	}
	r, _ := http.NewRequest("GET", "http://foo.com/", nil)
	for _, tt := range tests {
		h := http.HandlerFunc(tt.h)
		rec := NewRecorder()
		h.ServeHTTP(rec, r)
		for _, check := range tt.checks {
			if err := check(rec); err != nil {
				t.Errorf("%s: %v", tt.name, err)
			}
		}
	}
}
