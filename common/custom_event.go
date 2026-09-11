// Copyright 2014 Manu Martinez-Almeida.  All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

package common

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
)

// Server-Sent Events.
// W3C Working Draft 29 October 2009
// http://www.w3.org/TR/2009/WD-eventsource-20091029/

var contentType = []string{"text/event-stream"}
var noCache = []string{"no-cache"}

// var fieldReplacer = strings.NewReplacer(
// 	"\n", "\\n",
// 	"\r", "\\r")

var dataReplacer = strings.NewReplacer(
	"\n", "\ndata:",
	"\r", "\\r")

// CustomEvent represents a server-sent event that can be streamed to clients.
// The fields map directly to the SSE specification, with Data carrying the message body.
type CustomEvent struct {
	Event string
	Id    string
	Retry uint
	Data  any
}

// encode writes one custom event to writer and returns a wrapped write failure.
func encode(writer io.Writer, event CustomEvent) error {
	return writeData(writer, event.Data)
}

// writeData formats an event payload, escapes embedded line breaks, and terminates SSE data frames.
func writeData(writer io.Writer, data any) error {
	formatted := fmt.Sprint(data)
	if err := writeCustomEventString(writer, "data", dataReplacer.Replace(formatted)); err != nil {
		return err
	}
	if strings.HasPrefix(formatted, "data") {
		if err := writeCustomEventString(writer, "delimiter", "\n\n"); err != nil {
			return err
		}
	}
	return nil
}

// writeCustomEventString writes one complete event segment and rejects truncated writes.
func writeCustomEventString(writer io.Writer, segment string, payload string) error {
	written, err := io.WriteString(writer, payload)
	if err != nil {
		return errors.Wrapf(err, "write custom event %s", segment)
	}
	if written != len(payload) {
		return errors.Wrapf(io.ErrShortWrite,
			"write custom event %s: wrote %d of %d bytes", segment, written, len(payload))
	}
	return nil
}

// Render applies the SSE headers and writes the event payload to the provided ResponseWriter.
// It returns any error produced while encoding the event data.
func (r CustomEvent) Render(w http.ResponseWriter) error {
	r.WriteContentType(w)
	return encode(w, r)
}

// WriteContentType sets the Content-Type and Cache-Control headers expected for SSE responses.
func (r CustomEvent) WriteContentType(w http.ResponseWriter) {
	header := w.Header()
	header["Content-Type"] = contentType

	if _, exist := header["Cache-Control"]; !exist {
		header["Cache-Control"] = noCache
	}
}
