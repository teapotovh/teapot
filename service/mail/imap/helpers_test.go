package imap_test

import (
	"bufio"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
)

type lineReader struct {
	reader *bufio.Reader
}

func newLineReader(r io.Reader) *lineReader {
	return &lineReader{reader: bufio.NewReader(r)}
}

func (r *lineReader) readLine() (string, error) {
	line, err := r.reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	return strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r"), nil
}

func writeLine(t *testing.T, conn net.Conn, line string) {
	t.Helper()

	if _, err := conn.Write([]byte(line + "\r\n")); err != nil {
		t.Fatalf("could not write line: %v", err)
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}
