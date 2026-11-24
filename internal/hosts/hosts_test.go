package hosts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func render(t *testing.T, cur string, names ...string) string {
	t.Helper()
	out, err := Render(cur, names)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestOnThenOffGivesBackTheExactFile(t *testing.T) {
	for name, original := range map[string]string{
		"plain":            "127.0.0.1 localhost\n# my own line\n",
		"trailing blanks":  "127.0.0.1 localhost\n\n\n",
		"crlf":             "127.0.0.1 localhost\r\n",
		"mixed endings":    "127.0.0.1 localhost\r\n::1 localhost\n",
		"empty":            "",
		"old block inside": "a\n# >>> fuck-let-me-work >>>\n127.0.0.1 old.com\n# <<< fuck-let-me-work <<<\nb\n",
	} {
		on := render(t, original, "x.com", "www.x.com")
		if !strings.Contains(on, "127.0.0.1 x.com") {
			t.Errorf("%s: block not added:\n%q", name, on)
		}
		want := strings.Replace(original, "# >>> fuck-let-me-work >>>\n127.0.0.1 old.com\n# <<< fuck-let-me-work <<<\n", "", 1)
		if off := render(t, on); off != want {
			t.Errorf("%s: off gave\n%q\nwant\n%q", name, off, want)
		}
	}
}

func TestReplaceAndStable(t *testing.T) {
	on := render(t, "a\n", "x.com")
	swapped := render(t, on, "y.com")
	if strings.Contains(swapped, "x.com") || !strings.Contains(swapped, "127.0.0.1 y.com") {
		t.Fatalf("block not replaced:\n%s", swapped)
	}
	if again := render(t, swapped, "y.com"); again != swapped {
		t.Fatalf("second render changed the file")
	}
	if strings.Count(render(t, "a\r\n", "x.com"), "\n") != strings.Count(render(t, "a\r\n", "x.com"), "\r\n") {
		t.Fatal("block does not use the file's CRLF endings")
	}
}

func TestStartMarkerWithoutEndIsAnError(t *testing.T) {
	broken := "a\n# >>> fuck-let-me-work >>>\nb\nc\n"
	if _, err := Render(broken, nil); err == nil {
		t.Fatal("want error, user lines after the marker would be lost")
	}
}

func TestApplyOnlyWritesOnChange(t *testing.T) {
	p := filepath.Join(t.TempDir(), "hosts")
	if err := os.WriteFile(p, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Off on a file without our block must not touch it, even without a final newline.
	if changed, err := Apply(p, nil); changed || err != nil {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	for i, want := range []bool{true, false} {
		changed, err := Apply(p, []string{"x.com"})
		if err != nil || changed != want {
			t.Fatalf("apply %d: changed=%v err=%v, want %v", i, changed, err, want)
		}
	}
}
