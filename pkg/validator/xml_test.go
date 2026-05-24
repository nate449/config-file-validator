package validator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- XMLValidator.ValidateXSD method (marker interface) ---

func Test_XMLValidatorValidateXSDMethod(t *testing.T) {
	t.Parallel()
	xsdFile := writeTestXSD(t)
	xmlDoc := []byte(`<?xml version="1.0"?>
<config>
  <host>db.example.com</host>
  <port>5432</port>
</config>`)
	valid, err := XMLValidator{}.ValidateXSD(xmlDoc, xsdFile)
	require.True(t, valid)
	require.NoError(t, err)
}

// --- ValidateXSD negative paths ---

func Test_ValidateXSD_NegativePaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		xml      []byte
		xsdSetup func(t *testing.T) string
		wantErr  string
	}{
		{
			name: "returns error when XSD schema file is malformed",
			xml:  []byte(`<?xml version="1.0"?><root/>`),
			xsdSetup: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				p := filepath.Join(dir, "bad.xsd")
				require.NoError(t, os.WriteFile(p, []byte("not valid xsd content"), 0600))
				return p
			},
			wantErr: "schema compilation error",
		},
		{
			name: "returns error when XSD schema file does not exist",
			xml:  []byte(`<?xml version="1.0"?><root/>`),
			xsdSetup: func(_ *testing.T) string {
				return "/nonexistent/path/schema.xsd"
			},
			wantErr: "schema compilation error",
		},
		{
			name: "returns error when XML cannot be parsed against a valid schema",
			xml:  []byte(`<?xml version="1.0"?><config><host>x</host><port>5432`),
			xsdSetup: func(t *testing.T) string {
				t.Helper()
				return writeTestXSD(t)
			},
			wantErr: "xml parse error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			xsdPath := tt.xsdSetup(t)
			valid, err := ValidateXSD(tt.xml, xsdPath)
			require.False(t, valid)
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

// --- extractXSDLocation edge cases ---

func Test_extractXSDLocation_ReturnsEmptyOnInputWithNoElements(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input []byte
	}{
		{
			name:  "empty input triggers decoder EOF before any element",
			input: []byte{},
		},
		{
			name:  "processing instruction only with no start element",
			input: []byte(`<?xml version="1.0"?>`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			loc, err := extractXSDLocation(tt.input)
			require.NoError(t, err)
			assert.Empty(t, loc, "expected no schema location from input with no elements")
		})
	}
}

// --- resolveXSDPath edge cases ---

// Test_resolveXSDPath_RelativePathExistsInCWD verifies that a relative
// schema location is returned as-is when the file already exists relative
// to the process working directory. t.Chdir is used to set the CWD,
// which precludes t.Parallel.
func Test_resolveXSDPath_RelativePathExistsInCWD(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "schema.xsd"), []byte("<xs:schema/>"), 0600))
	t.Chdir(dir)

	result := resolveXSDPath("schema.xsd", "/some/other/dir/file.xml")
	assert.Equal(t, "schema.xsd", result)
}

// --- cleanXSDError edge cases ---

func Test_cleanXSDError_ReturnsInputUnchangedWhenPatternDoesNotMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "plain error message without line info",
			input: "element 'port' is not valid",
		},
		{
			name:  "error with line prefix but missing required format parts",
			input: "line 5: something went wrong",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := cleanXSDError(tt.input)
			assert.Equal(t, tt.input, result, "non-matching input should be returned verbatim")
		})
	}
}

// NOTE: xml.go line 86 is intentionally left uncovered. It is a defensive
// fallback that executes only when helium's xsd.Validator.Validate returns
// a non-nil error yet the ErrorCollector contains zero errors. This
// condition depends on internal library behavior that cannot be triggered
// reliably from tests without mocking the helium library.
