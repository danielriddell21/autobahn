package game

import (
	"os"
	"path/filepath"
	"testing"
)

// careerFileWith puts content where the career file lives and returns its path.
func careerFileWith(t *testing.T, content string) string {
	t.Helper()
	inTempConfig(t)
	path, err := careerPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func fileIs(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Errorf("the career file was rewritten: %q, want %q", got, want)
	}
}

func TestNewShellNeverClobbersACareerItCouldNotUse(t *testing.T) {
	const newer = `{"version":99,"banked":777777,"earnings":5,"cars":[3],"paints":[2]}`
	tests := []struct{ name, file string }{
		{"corrupt", "{not json"},
		{"truncated", `{"version":1,"banked":7777`},
		{"newer version", newer},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := careerFileWith(t, tt.file)
			sh := NewShell(Options{})
			fileIs(t, path, tt.file)
			if sh.notice == "" {
				t.Error("the player was not told their career was not loaded")
			}
			// Later saves in the same session must not clobber it either.
			sh.career.Earn(10)
			if err := sh.career.Save(); err == nil {
				t.Error("Save succeeded on a career whose file could not be read")
			}
			sh.saveCareer()
			fileIs(t, path, tt.file)
		})
	}
}

func TestNewShellLeavesAnUnreadableCareerAlone(t *testing.T) {
	inTempConfig(t)
	path, err := careerPath()
	if err != nil {
		t.Fatal(err)
	}
	// A directory where the file should be: ReadFile fails, and not with
	// "does not exist".
	if err := os.MkdirAll(filepath.Join(path, "keep"), 0o750); err != nil {
		t.Fatal(err)
	}
	sh := NewShell(Options{})
	if !sh.career.keep {
		t.Error("an unreadable career was not marked as not ours to replace")
	}
	if err := sh.career.Save(); err == nil {
		t.Error("Save succeeded on an unreadable career")
	}
	if fi, err := os.Stat(path); err != nil || !fi.IsDir() {
		t.Errorf("the unreadable career was replaced: %v", err)
	}
}

func TestNewShellWritesBackOnlyAMigratedCareer(t *testing.T) {
	t.Run("v0 is migrated and written once", func(t *testing.T) {
		path := careerFileWith(t, `{"banked":4321}`)
		NewShell(Options{})
		got := LoadCareer()
		if got.Banked != 4321 || got.Version != careerVersion {
			t.Errorf("migrated career is %+v", got)
		}
		data, _ := os.ReadFile(path)
		if string(data) == `{"banked":4321}` {
			t.Error("a v0 file was not written back migrated")
		}
	})
	t.Run("a current file is not written", func(t *testing.T) {
		const clean = `{"version":1,"banked":9,"earnings":77,"cars":[3],"paints":[2]}`
		path := careerFileWith(t, clean)
		NewShell(Options{})
		fileIs(t, path, clean)
	})
	t.Run("a first run writes nothing", func(t *testing.T) {
		inTempConfig(t)
		path, _ := careerPath()
		NewShell(Options{})
		if _, err := os.Stat(path); err == nil {
			t.Error("a first run wrote a career file")
		}
	})
}
