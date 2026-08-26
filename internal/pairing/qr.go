package pairing

import "github.com/skip2/go-qrcode"

// RenderQRTerminal renders text as a QR code using half-height Unicode block
// characters, suitable for printing directly to a console.
func RenderQRTerminal(text string) (string, error) {
	q, err := qrcode.New(text, qrcode.Medium)
	if err != nil {
		return "", err
	}
	return q.ToSmallString(false), nil
}
