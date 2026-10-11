package notify

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIsSecretShapedKeyMatchesSPEC64Pattern(t *testing.T) {
	for key, want := range map[string]bool{
		"API_TOKEN": true, "client_secret": true, "AWS_KEY": true, "DB_PASSWORD": true,
		"my.credential": true, "REGION": false, "PATH": false, "": false,
	} {
		if got := IsSecretShapedKey(key); got != want {
			t.Errorf("IsSecretShapedKey(%q) = %v, want %v", key, got, want)
		}
	}
}

// The allocation-free ASCII path must agree with the upper-casing reference
// (SPEC §6.4: case-insensitive substring) on mixed case, near misses and keys
// with non-ASCII letters, which take the Unicode path.
func TestIsSecretShapedKeyAgreesWithTheUpperCasingReference(t *testing.T) {
	for _, key := range []string{
		"ToKeN", "pAsSwOrD_x", "x-secret-y", "SECRE", "TOKE", "KE", "k_e_y", "credentia",
		"CREDENTIALS", "naïve_key", "ſecret", "tokën", "ΚEY", "a.b-c", "key",
	} {
		upper := strings.ToUpper(key)
		want := false
		for _, substr := range secretShapedKeySubstrings {
			want = want || strings.Contains(upper, substr)
		}
		if got := IsSecretShapedKey(key); got != want {
			t.Errorf("IsSecretShapedKey(%q) = %v, want %v", key, got, want)
		}
	}
}

func TestRedactMasksSecretPairsAndSessionValues(t *testing.T) {
	env := map[string]string{"API_TOKEN": "tok-9f2", "REGION": "eu-north-7", "N": "12", "ONE": "Z", "EMPTY": "", "LONG": "eu-north-7-long"}
	for text, want := range map[string]string{
		`TOKEN=abc and "x"`:                     "TOKEN=" + MaskedPlaceholder + ` and "x"`,
		`password: 'hun ter2' ok`:               "password: " + MaskedPlaceholder + " ok",
		`secret = "a b"`:                        "secret = " + MaskedPlaceholder,
		"build=ok 12 steps":                     "build=ok " + MaskedPlaceholder + " steps",
		"short Z value":                         "short " + MaskedPlaceholder + " value",
		"nothing shaped like a value":           "nothing shaped like a value",
		"region eu-north-7 and eu-north-7-long": "region " + MaskedPlaceholder + " and " + MaskedPlaceholder,
		"used tok-9f2 here":                     "used " + MaskedPlaceholder + " here",
	} {
		if got := Redact(text, env); got != want {
			t.Errorf("Redact(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestTruncateCutsOnARuneBoundary(t *testing.T) {
	if got := truncate("abc", 10); got != "abc" {
		t.Errorf("short text changed: %q", got)
	}
	if got := truncate("abc", 0); got != "abc" {
		t.Errorf("zero limit must mean no cap, got %q", got)
	}
	got := truncate(strings.Repeat("é", 5), 3)
	if got != "é…" {
		t.Errorf("truncate = %q, want a whole rune then the marker", got)
	}
}

func TestTailBufferKeepsTheLastBytesOnARuneStart(t *testing.T) {
	b := newTailBuffer(5)
	_, _ = b.Write([]byte("héllo wörld"))
	got, dropped := b.tail()
	if !dropped || got != "örld" {
		t.Errorf("tail = %q dropped %v, want the last whole runes", got, dropped)
	}
	if got, trunc := capTail("abcdef", 3, false); got != "def" || !trunc {
		t.Errorf("capTail = %q %v", got, trunc)
	}
	if got, trunc := capTail("éé", 3, false); got != "é" || !trunc {
		t.Errorf("capTail rune = %q %v", got, trunc)
	}
}

const (
	testJWT = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"
	// The same token with the JOSE header spaced the way JSON allows: its first
	// bytes encode to eyAi, ewog and IHsi, not eyJ.
	testJWTSpaced  = "eyAiYWxnIjogIkhTMjU2IiwgInR5cCI6ICJKV1QiIH0.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"
	testJWTNewline = "ewogICJhbGciOiAiSFMyNTYiCn0.eyJzdWIiOiIxMjM0NTY3ODkwIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"
	testJWTLeading = "IHsiYWxnIjoiUlMyNTYifSA.eyJzdWIiOiIxIn0.c2lnbmF0dXJlLXNlZ21lbnQ"
	testGHP        = "ghp_aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789"                   //nolint:gosec // G101: a fixture shaped like a credential on purpose, to prove the mask
	testPAT        = "github_pat_11ABCDEFG0aBcDeFgHiJkL_mNoPqRsTuVwXyZ0123456789" //nolint:gosec // G101: a fixture shaped like a credential on purpose, to prove the mask
	testAWSKey     = "AKIAIOSFODNN7EXAMPLE"                                       //nolint:gosec // G101: a fixture shaped like a credential on purpose, to prove the mask
	testSK         = "sk-proj-AbCdEfGhIjKlMnOp1234"
)

// Each secret shape of SPEC §10.1 and the lines next to them that must not be
// touched. A leak names the substring that must not survive.
var maskShapeCases = []struct {
	name, in, want, leak string
}{
	{"json quoted key", `{"api_key": "sk-x"}`, `{"api_key": ` + MaskedPlaceholder + `}`, "sk-x"},
	{"json quoted key no space", `"password":"hunter2"`, `"password":` + MaskedPlaceholder, "hunter2"},
	{"single quoted key", `{'client_secret' : 'a b'} next`, `{'client_secret' : ` + MaskedPlaceholder + `} next`, "a b"},
	{"authorization bearer", "Authorization: Bearer abc.def-123", "Authorization: Bearer " + MaskedPlaceholder, "abc.def-123"},
	{"bearer alone", "curl -H Bearer tok_8f3a2c9d now", "curl -H Bearer " + MaskedPlaceholder + " now", "tok_8f3a2c9d"},
	{"bearer lower case", "sent bearer zzTOPsecret1 ok", "sent bearer " + MaskedPlaceholder + " ok", "zzTOPsecret1"},
	{"flag token space", "run --token abc123 --verbose", "run --token " + MaskedPlaceholder + " --verbose", "abc123"},
	{"flag token equals", "run --token=abc123 --verbose", "run --token=" + MaskedPlaceholder + " --verbose", "abc123"},
	{"flag password space", "run --password abc123 go", "run --password " + MaskedPlaceholder + " go", "abc123"},
	{"flag password equals", "run --password=abc123 go", "run --password=" + MaskedPlaceholder + " go", "abc123"},
	{"flag api-key space", "run --api-key abc123 go", "run --api-key " + MaskedPlaceholder + " go", "abc123"},
	{"flag api-key equals", "run --api-key=abc123 go", "run --api-key=" + MaskedPlaceholder + " go", "abc123"},
	{"flag secret quoted", `run --client_secret "a b c" go`, "run --client_secret " + MaskedPlaceholder + " go", "a b c"},
	{"sk prefix", "key is " + testSK + " ok", "key is " + MaskedPlaceholder + " ok", testSK},
	{"ghp prefix", "pushed with " + testGHP + ".", "pushed with " + MaskedPlaceholder + ".", testGHP},
	{"github_pat prefix", "pat " + testPAT + " end", "pat " + MaskedPlaceholder + " end", testPAT},
	{"AKIA prefix", "aws " + testAWSKey + " end", "aws " + MaskedPlaceholder + " end", testAWSKey},
	{"jwt", "got " + testJWT + " back", "got " + MaskedPlaceholder + " back", "SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"},
	{"jwt spaced header", "got " + testJWTSpaced + " back", "got " + MaskedPlaceholder + " back", "SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"},
	{"jwt newline header", "tok=\n" + testJWTNewline + " back", "tok=\n" + MaskedPlaceholder + " back", "SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"},
	{"jwt leading-space header", "x " + testJWTLeading + ", y", "x " + MaskedPlaceholder + ", y", "c2lnbmF0dXJl"},
	{"jwt spaced header in quotes", `{"note": "` + testJWTSpaced + `"}`, `{"note": "` + MaskedPlaceholder + `"}`, "SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"},
	{"dotted prose", "see file.name.txt and e30.e30.x and aGVsbG8.d29ybGQ.IQ end", "see file.name.txt and e30.e30.x and aGVsbG8.d29ybGQ.IQ end", ""},
	{"non-object json header", "WzEsMl0.eyJhIjoxfQ.c2ln MQ.Mg.Mw", "WzEsMl0.eyJhIjoxfQ.c2ln MQ.Mg.Mw", ""},
	{"dotted words around", "a.b.c, foo.bar.baz. x.y", "a.b.c, foo.bar.baz. x.y", ""},
	// A backslash escapes the byte after it inside a quoted value: the secret
	// is masked through its real closing quote and no suffix survives.
	{"json escaped quote", `{"api_key": "ab\"cd-LEAK"} tail`, `{"api_key": ` + MaskedPlaceholder + `} tail`, "LEAK"},
	{"json escaped backslash then close", `"password":"a\\" tail`, `"password":` + MaskedPlaceholder + ` tail`, ""},
	{"json backslash backslash quote", `"secret":"a\\\"LEAK" tail`, `"secret":` + MaskedPlaceholder + ` tail`, "LEAK"},
	{"single quoted key escaped quote", `{'client_secret': 'it\'s LEAK'} next`, `{'client_secret': ` + MaskedPlaceholder + `} next`, "LEAK"},
	{"key value escaped quote", `API_TOKEN="a\"b LEAK" rest`, `API_TOKEN=` + MaskedPlaceholder + ` rest`, "LEAK"},
	{"flag escaped quote space", `run --token "a\"b LEAK" go`, "run --token " + MaskedPlaceholder + " go", "LEAK"},
	{"flag escaped quote equals", `run --password='x\'y LEAK' go`, "run --password=" + MaskedPlaceholder + " go", "LEAK"},
	{"flag escaped backslash", `run --api-key "a\\" go`, "run --api-key " + MaskedPlaceholder + " go", ""},
	{"escaped quote in a non-secret", `{"author": "a\"b c"}`, `{"author": "a\"b c"}`, ""},
	// Another shape inside a quoted secret value is part of that value: it
	// never takes the closing quote with it and leaves a suffix in the clear.
	{"flag value holding bearer", `--password "prefix\" EXPOSED_SUFFIX Bearer last" KEEP_AFTER`, "--password " + MaskedPlaceholder + " KEEP_AFTER", "EXPOSED_SUFFIX"},
	{"pair value holding bearer", `API_TOKEN="prefix\" EXPOSED_SUFFIX Bearer last" KEEP_AFTER`, "API_TOKEN=" + MaskedPlaceholder + " KEEP_AFTER", "EXPOSED_SUFFIX"},
	{"quoted key value holding bearer", `"password": "prefix\" EXPOSED_SUFFIX Bearer last" KEEP_AFTER`, `"password": ` + MaskedPlaceholder + " KEEP_AFTER", "EXPOSED_SUFFIX"},
	{"flag value holding a quoted key", `--password "a\" LEAK \"token\": b" KEEP`, "--password " + MaskedPlaceholder + " KEEP", "LEAK"},
	{"pair value holding a flag", `API_KEY='a\' LEAK --token b' KEEP`, "API_KEY=" + MaskedPlaceholder + " KEEP", "LEAK"},
	{"bearer token running into a pair", `Bearer API_TOKEN="a\" LEAK" KEEP`, "Bearer " + MaskedPlaceholder + " KEEP", "LEAK"},
	{"secret pair as a non-secret pair's value", "error: GITHUB_TOKEN=abc123 rejected", "error: GITHUB_TOKEN=" + MaskedPlaceholder + " rejected", "abc123"},
	{"quoted bearer token", `Authorization: Bearer "a\" LEAK" KEEP`, "Authorization: Bearer " + MaskedPlaceholder + " KEEP", "LEAK"},
	// Already masked before: they stay masked.
	{"export", "export GITHUB_TOKEN=x", "export GITHUB_TOKEN=" + MaskedPlaceholder, "=x"},
	{"aws colon", "AWS_SECRET_ACCESS_KEY: x", "AWS_SECRET_ACCESS_KEY: " + MaskedPlaceholder, ": x"},
	// Not secrets: byte-identical.
	{"tokens per minute", "--tokens-per-minute 5", "--tokens-per-minute 5", ""},
	{"max tokens", "run --max-tokens 100 --keyboard us", "run --max-tokens 100 --keyboard us", ""},
	{"author", "author: bob", "author: bob", ""},
	{"plain json", `{"author": "bob", "name": "x"}`, `{"author": "bob", "name": "x"}`, ""},
	{"short prefix words", "task-force disk-usage sk-learn ask-sk-x1234567890", "task-force disk-usage sk-learn ask-sk-x1234567890", ""},
	{"version dots", "v1.2.3 a.b.c eyJ.x", "v1.2.3 a.b.c eyJ.x", ""},
	{"flag then flag", "--token --verbose", "--token --verbose", ""},
	{"bearer in a word", "forbearer x", "forbearer x", ""},
}

func TestMaskSecretAssignmentsMasksEachSecretShape(t *testing.T) {
	for _, tc := range maskShapeCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := maskSecretAssignments(tc.in); got != tc.want {
				t.Errorf("maskSecretAssignments(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if got := Redact(tc.in, nil); got != tc.want {
				t.Errorf("Redact(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if tc.leak != "" && strings.Contains(Redact(tc.in, nil), tc.leak) {
				t.Errorf("Redact(%q) still carries %q", tc.in, tc.leak)
			}
		})
	}
}

// The new passes stay linear: a megabyte of near-misses and real shapes is
// masked and costs about what reading it does.
func TestMaskSecretAssignmentsStaysLinearOnAVeryLongText(t *testing.T) {
	text := strings.Repeat("--token abc Bearer xyz sk-aaaaaaaaaa eyJ.a.b \"api_key\": \"v\" --max-tokens 5 ::: ", 1<<14)
	start := time.Now()
	got := maskSecretAssignments(text)
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("a %d-byte text took %v", len(text), elapsed)
	}
	for _, leak := range []string{"abc", "xyz", "aaaaaaaaaa", `"v"`} {
		if strings.Contains(got, leak) {
			t.Fatalf("%q survived in a %d-byte text", leak, len(text))
		}
	}
	if strings.Count(got, "--max-tokens 5") != 1<<14 {
		t.Fatal("a non-secret flag was altered or lost")
	}
}

// Every quoted secret value, whichever shape carries it and whichever quote
// it uses, is masked whole through its real closing quote, whatever other
// shape sits inside it: the inner shape is last, so masking it on its own
// would take the closing quote and leave the suffix in the clear.
func TestMaskSecretAssignmentsMasksAQuotedValueWholeWhateverShapeItHolds(t *testing.T) {
	outers := []string{"--password ", "--api-key=", "API_TOKEN=", "client_secret: ", "|password_|: ", "Bearer ", "error: API_KEY="}
	inners := []string{"Bearer tok", "--token tok", "PASSWORD=tok", `\|token\|: tok`, "x " + testSK, "x " + testJWT}
	for _, quote := range []string{`"`, "'"} {
		for _, outer := range outers {
			for _, inner := range inners {
				prefix := strings.ReplaceAll(outer, "|", quote)
				in := prefix + strings.ReplaceAll(quote+`pre\| LEAK `+inner+quote, "|", quote) + " KEEP"
				want := prefix + MaskedPlaceholder + " KEEP"
				if got := Redact(in, nil); got != want {
					t.Errorf("Redact(%q) = %q, want %q", in, got, want)
				}
			}
		}
	}
}

// One shape per field the script sees: the message in DECK_EVENT_MESSAGE and
// on stdin, and the script's own output in the stored tail, all through the
// same masking.
func TestSpawnMasksEscapedQuotedValuesInEnvPayloadAndStoredOutput(t *testing.T) {
	path, dir := captureScript(t, `echo "out: $DECK_EVENT_MESSAGE"; echo 'err: {"password": "x\"OUTLEAK y"} --token "q\"FLAGLEAK z" end' >&2`)
	req := baseRequest(path)
	req.Event.Message = `got {"api_key": "ab\"MSGLEAK"} and --secret "s\"ARGLEAK t" done`
	res, err := Spawn(context.Background(), req)
	if err != nil || res.Failed() {
		t.Fatalf("Spawn = %+v, %v", res, err)
	}
	env := envMap(read(t, filepath.Join(dir, "env")))
	var p struct{ Event struct{ Message string } }
	if err := json.Unmarshal([]byte(read(t, filepath.Join(dir, "stdin"))), &p); err != nil {
		t.Fatal(err)
	}
	want := `got {"api_key": ` + MaskedPlaceholder + `} and --secret ` + MaskedPlaceholder + " done"
	if env["DECK_EVENT_MESSAGE"] != want || p.Event.Message != want {
		t.Errorf("message env %q, stdin %q, want %q", env["DECK_EVENT_MESSAGE"], p.Event.Message, want)
	}
	for _, leak := range []string{"MSGLEAK", "ARGLEAK", "OUTLEAK", "FLAGLEAK", " y\"", " z\"", " t\""} {
		if strings.Contains(res.Output, leak) || strings.Contains(env["DECK_EVENT_MESSAGE"], leak) || strings.Contains(p.Event.Message, leak) {
			t.Errorf("a surface carries %q: output %q", leak, res.Output)
		}
	}
	if !strings.Contains(res.Output, `"password": `+MaskedPlaceholder) || !strings.Contains(res.Output, "--token "+MaskedPlaceholder+" end") {
		t.Errorf("stored output = %q, want the masked shapes in the tail", res.Output)
	}
}

// A quoted secret value holding another shape is masked whole on every
// surface the script sees: DECK_EVENT_MESSAGE, the stdin JSON and the stored
// output tail.
func TestSpawnMasksAQuotedSecretHoldingAnotherShapeOnEverySurface(t *testing.T) {
	path, dir := captureScript(t, `echo "out: $DECK_EVENT_MESSAGE"; echo 'err: API_TOKEN="p\" OUTLEAK Bearer last" KEEP_OUT {"secret": "q\" JSONLEAK --token z"} done' >&2`)
	req := baseRequest(path)
	req.Event.Message = `--password "prefix\" MSGLEAK Bearer last" KEEP_AFTER and Bearer "t\" TOKLEAK" end`
	res, err := Spawn(context.Background(), req)
	if err != nil || res.Failed() {
		t.Fatalf("Spawn = %+v, %v", res, err)
	}
	env := envMap(read(t, filepath.Join(dir, "env")))
	var p struct{ Event struct{ Message string } }
	if err := json.Unmarshal([]byte(read(t, filepath.Join(dir, "stdin"))), &p); err != nil {
		t.Fatal(err)
	}
	want := "--password " + MaskedPlaceholder + " KEEP_AFTER and Bearer " + MaskedPlaceholder + " end"
	if env["DECK_EVENT_MESSAGE"] != want || p.Event.Message != want {
		t.Errorf("message env %q, stdin %q, want %q", env["DECK_EVENT_MESSAGE"], p.Event.Message, want)
	}
	for _, leak := range []string{"MSGLEAK", "TOKLEAK", "OUTLEAK", "JSONLEAK", "last"} {
		if strings.Contains(res.Output, leak) || strings.Contains(env["DECK_EVENT_MESSAGE"], leak) || strings.Contains(p.Event.Message, leak) {
			t.Errorf("a surface carries %q: output %q", leak, res.Output)
		}
	}
	// The tail is masked once more as a whole, so the brace glued to the
	// masked JSON value goes with it.
	for _, kept := range []string{"out: " + want, "err: API_TOKEN=" + MaskedPlaceholder + " KEEP_OUT", `{"secret": ` + MaskedPlaceholder + " done"} {
		if !strings.Contains(res.Output, kept) {
			t.Errorf("stored output = %q, want it to hold %q", res.Output, kept)
		}
	}
}

func TestSpawnMasksTheNewShapesInEnvPayloadAndStoredOutput(t *testing.T) {
	const secret = testGHP
	path, dir := captureScript(t, `echo "out: $DECK_EVENT_MESSAGE"; echo "err: curl -H 'Authorization: Bearer tok-leak-12345' --password hunter2x" >&2`)
	req := baseRequest(path)
	req.Event.Message = `called with {"api_key": "sk-x"} and ` + secret + " --token abc123"
	res, err := Spawn(context.Background(), req)
	if err != nil || res.Failed() {
		t.Fatalf("Spawn = %+v, %v", res, err)
	}
	env := envMap(read(t, filepath.Join(dir, "env")))
	var p struct{ Event struct{ Message string } }
	if err := json.Unmarshal([]byte(read(t, filepath.Join(dir, "stdin"))), &p); err != nil {
		t.Fatal(err)
	}
	want := `called with {"api_key": ` + MaskedPlaceholder + `} and ` + MaskedPlaceholder + " --token " + MaskedPlaceholder
	if env["DECK_EVENT_MESSAGE"] != want || p.Event.Message != want {
		t.Errorf("message env %q, stdin %q, want %q", env["DECK_EVENT_MESSAGE"], p.Event.Message, want)
	}
	for _, leak := range []string{secret, "sk-x", "abc123", "tok-leak-12345", "hunter2x"} {
		if strings.Contains(res.Output, leak) {
			t.Errorf("stored output tail carries %q: %q", leak, res.Output)
		}
	}
	// The tail is masked once more as a whole, so a closing brace or quote
	// glued to a masked value goes with it: only the masked shapes are asserted.
	if !strings.Contains(res.Output, `"api_key": `+MaskedPlaceholder) || !strings.Contains(res.Output, "--token "+MaskedPlaceholder) ||
		!strings.Contains(res.Output, "Bearer "+MaskedPlaceholder) ||
		!strings.Contains(res.Output, "--password "+MaskedPlaceholder) {
		t.Errorf("stored output = %q, want the masked shapes in the tail", res.Output)
	}
}

// A JWT whose JOSE header is spaced the way JSON allows is masked in the
// message (env and stdin) and in the stored output tail, while dotted prose
// next to it is untouched.
func TestSpawnMasksAWhitespaceHeaderJWTInEnvPayloadAndStoredOutput(t *testing.T) {
	path, dir := captureScript(t, `echo "out: $DECK_EVENT_MESSAGE"; echo "err: jwt `+testJWTNewline+` file.name.txt" >&2`)
	req := baseRequest(path)
	req.Event.Message = "token " + testJWTSpaced + " in file.name.txt"
	res, err := Spawn(context.Background(), req)
	if err != nil || res.Failed() {
		t.Fatalf("Spawn = %+v, %v", res, err)
	}
	env := envMap(read(t, filepath.Join(dir, "env")))
	var p struct{ Event struct{ Message string } }
	if err := json.Unmarshal([]byte(read(t, filepath.Join(dir, "stdin"))), &p); err != nil {
		t.Fatal(err)
	}
	want := "token " + MaskedPlaceholder + " in file.name.txt"
	if env["DECK_EVENT_MESSAGE"] != want || p.Event.Message != want {
		t.Errorf("message env %q, stdin %q, want %q", env["DECK_EVENT_MESSAGE"], p.Event.Message, want)
	}
	for _, leak := range []string{testJWTSpaced, testJWTNewline, "SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"} {
		if strings.Contains(res.Output, leak) {
			t.Errorf("stored output tail carries %q: %q", leak, res.Output)
		}
	}
	if !strings.Contains(res.Output, "jwt "+MaskedPlaceholder+" file.name.txt") {
		t.Errorf("stored output = %q, want the masked JWT and the prose kept", res.Output)
	}
}
