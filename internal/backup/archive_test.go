package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"strings"
	"testing"
	"web-panel/internal/errorpages"

	"web-panel/internal/domain"
)

func TestExtractRejectsUnsafeArchives(t *testing.T) {
	state, _ := json.Marshal(domain.Default())
	for _, test := range []struct {
		name string
		typ  byte
		size int64
	}{{"../escape", tar.TypeReg, 1}, {"/absolute", tar.TypeReg, 1}, {"sites/" + domain.NewID() + "/link", tar.TypeSymlink, 0}, {"unknown", tar.TypeReg, 1}, {"openresty/root/default/evil.txt", tar.TypeReg, 1}, {"openresty/root/default/404.html", tar.TypeSymlink, 0}, {"state.json", tar.TypeReg, 1}, {"sites/" + domain.NewID() + "/bomb", tar.TypeReg, MaxFileSize + 1}} {
		t.Run(test.name, func(t *testing.T) {
			var b bytes.Buffer
			gz := gzip.NewWriter(&b)
			tw := tar.NewWriter(gz)
			tw.WriteHeader(&tar.Header{Name: "state.json", Mode: 0600, Size: int64(len(state))})
			tw.Write(state)
			tw.WriteHeader(&tar.Header{Name: test.name, Typeflag: test.typ, Mode: 0600, Size: test.size})
			if test.size == 1 {
				tw.Write([]byte("x"))
			}
			tw.Close()
			gz.Close()
			if _, err := Extract(&b, t.TempDir()); err == nil {
				t.Fatal("unsafe archive accepted")
			}
		})
	}
}

func TestLegacyArchiveSeparatesHTMLFromState(t *testing.T) {
	data, _ := json.Marshal(domain.Default())
	var legacy map[string]any
	json.Unmarshal(data, &legacy)
	legacy["schema"] = 1
	legacy["pages"] = map[string]string{"404": "legacy archive HTML"}
	data, _ = json.Marshal(legacy)
	var buffer bytes.Buffer
	gz := gzip.NewWriter(&buffer)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "state.json", Mode: 0600, Size: int64(len(data))})
	tw.Write(data)
	tw.Close()
	gz.Close()
	stage := t.TempDir()
	state, err := Extract(&buffer, stage)
	if err != nil {
		t.Fatal(err)
	}
	pages, err := errorpages.New(stage).Read("")
	if err != nil || pages.Pages["404"] != "legacy archive HTML" {
		t.Fatal("legacy HTML not imported", err)
	}
	current, _ := json.Marshal(state)
	if state.Schema != 2 || strings.Contains(string(current), "legacy archive HTML") || strings.Contains(string(current), `"pages"`) {
		t.Fatal("HTML remained in controller state")
	}
}

func TestExtractChecksEntireCompressedEnvelope(t *testing.T) {
	state, _ := json.Marshal(domain.Default())
	var tarData bytes.Buffer
	tw := tar.NewWriter(&tarData)
	if err := tw.WriteHeader(&tar.Header{Name: "state.json", Mode: 0600, Size: int64(len(state))}); err != nil {
		t.Fatal(err)
	}
	tw.Write(state)
	tw.Close()
	compress := func(data []byte) []byte {
		var output bytes.Buffer
		writer := gzip.NewWriter(&output)
		writer.Write(data)
		writer.Close()
		return output.Bytes()
	}
	valid := compress(append(append([]byte{}, tarData.Bytes()...), make([]byte, 512)...))
	corrupt := append([]byte{}, valid...)
	corrupt[len(corrupt)-1] ^= 1
	cases := map[string][]byte{
		"checksum after padding": corrupt,
		"hidden tar content":     compress(append(append([]byte{}, tarData.Bytes()...), []byte("hidden")...)),
		"second gzip member":     append(append([]byte{}, valid...), compress([]byte("hidden"))...),
		"trailing raw content":   append(append([]byte{}, valid...), []byte("hidden")...),
		"oversized tar padding":  compress(append(append([]byte{}, tarData.Bytes()...), make([]byte, (1<<20)+1)...)),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Extract(bytes.NewReader(data), t.TempDir()); err == nil {
				t.Fatal("invalid compressed envelope accepted")
			}
		})
	}
	if _, err := Extract(bytes.NewReader(valid), t.TempDir()); err != nil {
		t.Fatal("valid tar padding rejected", err)
	}
}

func TestExtractRejectsEmptyPageBeforeRestore(t *testing.T) {
	state, _ := json.Marshal(domain.Default())
	var data bytes.Buffer
	gz := gzip.NewWriter(&data)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "state.json", Mode: 0600, Size: int64(len(state))})
	tw.Write(state)
	tw.WriteHeader(&tar.Header{Name: "openresty/root/default/404.html", Mode: 0600, Typeflag: tar.TypeReg})
	tw.Close()
	gz.Close()
	if _, err := Extract(&data, t.TempDir()); err == nil {
		t.Fatal("empty page reached restore commit")
	}
}
