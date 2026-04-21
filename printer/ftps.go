package printer

import (
	"crypto/tls"
	"fmt"
	"io"
	"time"

	"github.com/jlaffaye/ftp"
)

// DownloadGCode opens a fresh FTPS connection to the printer, retrieves the file at remotePath,
// and returns a ReadCloser. The caller is responsible for closing the returned reader.
// Each call establishes its own connection; connections are not pooled.
func DownloadGCode(cfg Config, remotePath string) (io.ReadCloser, error) {
	conn, err := ftp.Dial(
		cfg.FTPSAddr(),
		ftp.DialWithTimeout(30*time.Second),
		ftp.DialWithTLS(&tls.Config{
			InsecureSkipVerify: true, //nolint:gosec // printer uses a self-signed certificate
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("ftps dial %s: %w", cfg.FTPSAddr(), err)
	}

	if err := conn.Login("bblp", cfg.AccessCode); err != nil {
		_ = conn.Quit()
		return nil, fmt.Errorf("ftps login: %w", err)
	}

	r, err := conn.Retr(remotePath)
	if err != nil {
		_ = conn.Quit()
		return nil, fmt.Errorf("ftps retrieve %s: %w", remotePath, err)
	}

	// Wrap the response so closing it also quits the FTP connection.
	return &ftpsReadCloser{Response: r, conn: conn}, nil
}

// ftpsReadCloser closes both the FTP response and the underlying connection on Close.
type ftpsReadCloser struct {
	*ftp.Response
	conn *ftp.ServerConn
}

func (f *ftpsReadCloser) Close() error {
	err := f.Response.Close()
	if quitErr := f.conn.Quit(); quitErr != nil && err == nil {
		err = quitErr
	}
	return err
}
