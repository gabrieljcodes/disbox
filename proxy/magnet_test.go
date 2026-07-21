package proxy

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"strings"
	"testing"
)

func TestExtractMagnetFromTorrentFile(t *testing.T) {
	// Construct a minimal valid .torrent bencoded structure:
	// d4:info d4:name 8:testfile 12:piece length i16384e 6:pieces 20:12345678901234567890 e e
	infoDict := []byte("d4:name8:testfile12:piece lengthi16384e6:pieces20:12345678901234567890e")
	torrentFile := bytes.Join([][]byte{
		[]byte("d4:info"),
		infoDict,
		[]byte("e"),
	}, nil)

	hash := sha1.Sum(infoDict)
	expectedHex := fmt.Sprintf("%x", hash)
	expectedMagnet := fmt.Sprintf("magnet:?xt=urn:btih:%s&dn=testfile", expectedHex)

	magnet, err := ExtractMagnetFromTorrentFile(torrentFile)
	if err != nil {
		t.Fatalf("ExtractMagnetFromTorrentFile failed: %v", err)
	}

	if magnet != expectedMagnet {
		t.Errorf("Expected magnet %q, got %q", expectedMagnet, magnet)
	}
}

func TestExtractMagnetInvalidData(t *testing.T) {
	_, err := ExtractMagnetFromTorrentFile([]byte("invalid bencode"))
	if err == nil {
		t.Error("Expected error for invalid bencode, got nil")
	}

	_, err = ExtractMagnetFromTorrentFile([]byte{})
	if err == nil {
		t.Error("Expected error for empty bencode, got nil")
	}

	_, err = ExtractMagnetFromTorrentFile([]byte("d8:announce10:http://test e"))
	if err == nil || !strings.Contains(err.Error(), "info") {
		t.Errorf("Expected missing info dictionary error, got %v", err)
	}
}
