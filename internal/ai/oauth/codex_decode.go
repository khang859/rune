package oauth

import "encoding/base64"

func jsonStdBase64Decode(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}
