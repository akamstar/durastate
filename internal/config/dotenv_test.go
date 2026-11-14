package config

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseDotenv(t *testing.T) {
	src := "\xef\xbb\xbf" + `# comment
PLAIN=value
SPACED =  padded value
EMPTY=
INLINE=abc # comment
HASH=a#b
TAB=x	# tab comment
export EXPORTED=yes
DOUBLE="a\tb\nc \"q\" \\ C:\path"
SINGLE='raw $X \n'
QUOTED_COMMENT="v" # comment
EQUALS=a=b=c
WIN_CRLF=crlf` + "\r\n" + `DUP=first
DUP=second
`
	got, errs := parseDotenv(".env", []byte(src))
	if err := errs.Err(); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"PLAIN":          "value",
		"SPACED":         "padded value",
		"EMPTY":          "",
		"INLINE":         "abc",
		"HASH":           "a#b",
		"TAB":            "x",
		"EXPORTED":       "yes",
		"DOUBLE":         "a\tb\nc \"q\" \\ C:\\path",
		"SINGLE":         `raw $X \n`,
		"QUOTED_COMMENT": "v",
		"EQUALS":         "a=b=c",
		"WIN_CRLF":       "crlf",
		"DUP":            "second",
	}
	if !reflect.DeepEqual(got, want) {
		for k, v := range want {
			if got[k] != v {
				t.Errorf("%s = %q, want %q", k, got[k], v)
			}
		}
		if len(got) != len(want) {
			t.Errorf("got %d variables, want %d: %v", len(got), len(want), got)
		}
	}
}

func TestParseDotenvErrors(t *testing.T) {
	src := "OK=1\nnot a pair\n1BAD=x\nBAD-NAME=x\nU=\"open\nT='a' b\n=v\n"
	_, errs := parseDotenv("app/.env", []byte(src))
	want := []string{
		"app/.env:2: invalid line (expected KEY=value)",
		`app/.env:3: invalid variable name "1BAD"`,
		`app/.env:4: invalid variable name "BAD-NAME"`,
		"app/.env:5: U: unterminated quoted value",
		"app/.env:6: T: unexpected characters after the closing quote",
		`app/.env:7: invalid variable name ""`,
	}
	got := strings.Split(errs.Err().Error(), "\n")
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func FuzzParseDotenv(f *testing.F) {
	f.Add([]byte("A=1\nB=\"x\\\"y\"\nC='z'\nexport D=e # c\n"))
	f.Add([]byte("X=\"\\"))
	f.Fuzz(func(t *testing.T, data []byte) {
		vars, _ := parseDotenv(".env", data)
		for k := range vars {
			if !envNameRe.MatchString(k) {
				t.Fatalf("invalid name %q accepted", k)
			}
		}
	})
}
