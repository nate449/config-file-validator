package reporter

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Shared test fixtures
var (
	validReport = Report{
		FileName: "good.xml",
		FilePath: "/fake/path/good.xml",
		IsValid:  true,
	}

	backslashReport = Report{
		FileName: "good.xml",
		FilePath: "\\fake\\path\\good.xml",
		IsValid:  true,
	}

	invalidReport = Report{
		FileName:         "bad.xml",
		FilePath:         "/fake/path/bad.xml",
		IsValid:          false,
		ValidationError:  errors.New("unable to parse bad.xml file"),
		ValidationErrors: []string{"unable to parse bad.xml file"},
	}

	multiLineErrorReport = Report{
		FileName:         "bad.xml",
		FilePath:         "/fake/path/bad.xml",
		IsValid:          false,
		ValidationError:  errors.New("unable to parse keys:\nkey1\nkey2"),
		ValidationErrors: []string{"unable to parse keys:\nkey1\nkey2"},
	}

	quietReport = Report{
		FileName: "good.xml",
		FilePath: "/fake/path/good.xml",
		IsValid:  true,
		IsQuiet:  true,
	}

	reportWithNotes = Report{
		FileName:         "noted.yaml",
		FilePath:         "/fake/path/noted.yaml",
		IsValid:          false,
		ValidationError:  errors.New("missing key"),
		ValidationErrors: []string{"missing key"},
		Notes:            []string{"consider adding default value"},
	}

	mixedReports = []Report{validReport, invalidReport, multiLineErrorReport}
)

// captureStdout redirects os.Stdout to a pipe, runs fn, and returns
// all bytes written to stdout during fn's execution.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)
	return buf.String()
}

// --- Stdout reporter tests ---

func Test_stdoutReport(t *testing.T) {
	output := captureStdout(t, func() {
		err := NewStdoutReporter("").Print(mixedReports)
		require.NoError(t, err)
	})
	assert.Contains(t, output, "/fake/path/good.xml")
	assert.Contains(t, output, "/fake/path/bad.xml")
	assert.Contains(t, output, "✓")
	assert.Contains(t, output, "×")
	assert.Contains(t, output, "unable to parse bad.xml file")
	assert.Contains(t, output, "unable to parse keys:")
}

func Test_stdoutReportQuiet(t *testing.T) {
	output := captureStdout(t, func() {
		err := NewStdoutReporter("").Print([]Report{quietReport})
		require.NoError(t, err)
	})
	assert.Empty(t, output)
}

func Test_stdoutReportToFile(t *testing.T) {
	t.Run("to dir", func(t *testing.T) {
		tmpDir := t.TempDir()
		err := NewStdoutReporter(tmpDir).Print([]Report{validReport})
		require.NoError(t, err)

		data, err := os.ReadFile(tmpDir + "/result.txt")
		require.NoError(t, err)
		content := string(data)
		assert.Contains(t, content, "/fake/path/good.xml")
		assert.Contains(t, content, "✓")
	})

	t.Run("to file", func(t *testing.T) {
		tmpDir := t.TempDir()
		outPath := tmpDir + "/my_output.txt"
		err := NewStdoutReporter(outPath).Print([]Report{validReport})
		require.NoError(t, err)

		data, err := os.ReadFile(outPath)
		require.NoError(t, err)
		assert.Contains(t, string(data), "/fake/path/good.xml")
	})

	t.Run("to bad path", func(t *testing.T) {
		err := NewStdoutReporter("/nonexistent/path/output").Print([]Report{validReport})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create a file")
	})
}

func Test_stdoutReportWithNotes(t *testing.T) {
	result := createStdoutReport([]Report{reportWithNotes}, 1)
	assert.Contains(t, result.Text, "/fake/path/noted.yaml")
	assert.Contains(t, result.Text, "missing key")
	assert.Contains(t, result.Text, "note:")
	assert.Contains(t, result.Text, "consider adding default value")
	assert.Equal(t, 0, result.Summary.Passed)
	assert.Equal(t, 1, result.Summary.Failed)
}

// --- JSON reporter tests ---

func Test_jsonReport(t *testing.T) {
	reports := []Report{validReport, invalidReport, backslashReport}
	output := captureStdout(t, func() {
		err := (&JSONReporter{}).Print(reports)
		require.NoError(t, err)
	})

	var parsed reportJSON
	require.NoError(t, json.Unmarshal([]byte(output), &parsed))
	require.Len(t, parsed.Files, 3)

	assert.Equal(t, "/fake/path/good.xml", parsed.Files[0].Path)
	assert.Equal(t, "passed", parsed.Files[0].Status)

	assert.Equal(t, "/fake/path/bad.xml", parsed.Files[1].Path)
	assert.Equal(t, "failed", parsed.Files[1].Status)
	assert.Contains(t, parsed.Files[1].Errors, "unable to parse bad.xml file")

	// Backslash path converted to forward slashes
	assert.Equal(t, "/fake/path/good.xml", parsed.Files[2].Path)
	assert.Equal(t, "passed", parsed.Files[2].Status)

	assert.Equal(t, 2, parsed.Summary.Passed)
	assert.Equal(t, 1, parsed.Summary.Failed)
}

func Test_jsonReportQuiet(t *testing.T) {
	output := captureStdout(t, func() {
		err := (&JSONReporter{}).Print([]Report{quietReport})
		require.NoError(t, err)
	})
	assert.Empty(t, output)
}

func Test_jsonReportToFile(t *testing.T) {
	tmpDir := t.TempDir()
	err := NewJSONReporter(tmpDir).Print([]Report{validReport})
	require.NoError(t, err)

	data, err := os.ReadFile(tmpDir + "/result.json")
	require.NoError(t, err)

	var parsed reportJSON
	require.NoError(t, json.Unmarshal(data, &parsed))
	require.Len(t, parsed.Files, 1)
	assert.Equal(t, "/fake/path/good.xml", parsed.Files[0].Path)
	assert.Equal(t, "passed", parsed.Files[0].Status)
	assert.Equal(t, 1, parsed.Summary.Passed)
	assert.Equal(t, 0, parsed.Summary.Failed)
}

func Test_jsonReportWithNotes(t *testing.T) {
	output := captureStdout(t, func() {
		err := (&JSONReporter{}).Print([]Report{reportWithNotes})
		require.NoError(t, err)
	})

	var parsed reportJSON
	require.NoError(t, json.Unmarshal([]byte(output), &parsed))
	require.Len(t, parsed.Files, 1)
	assert.Equal(t, "failed", parsed.Files[0].Status)
	assert.Contains(t, parsed.Files[0].Errors, "missing key")
	assert.Contains(t, parsed.Files[0].Notes, "consider adding default value")
}

// --- JUnit reporter tests ---

func Test_junitReport(t *testing.T) {
	reports := []Report{validReport, backslashReport, {
		FileName:         "bad.xml",
		FilePath:         "/fake/path/bad.json",
		IsValid:          false,
		ValidationError:  errors.New("Incorrect characters '<' and '</>` found in file"),
		ValidationErrors: []string{"Incorrect characters '<' and '</>` found in file"},
	}}
	output := captureStdout(t, func() {
		err := (JunitReporter{}).Print(reports)
		require.NoError(t, err)
	})
	assert.Contains(t, output, `<?xml version="1.0" encoding="UTF-8"?>`)
	assert.Contains(t, output, "config-file-validator")
	assert.Contains(t, output, "/fake/path/good.xml")
	assert.Contains(t, output, "/fake/path/bad.json")
	assert.Contains(t, output, "&lt;")
	assert.Contains(t, output, "failure")
}

func Test_junitReportQuiet(t *testing.T) {
	output := captureStdout(t, func() {
		err := (JunitReporter{}).Print([]Report{quietReport})
		require.NoError(t, err)
	})
	assert.Empty(t, output)
}

func Test_junitGetReport(t *testing.T) {
	// Property with TextValue should fail
	prop1 := Property{Name: "property1", Value: "value", TextValue: "text value"}
	ts := Testsuites{Name: "cfv", Tests: 1, Testsuites: []Testsuite{
		{Name: "cfv", Errors: 0, Properties: &[]Property{prop1}},
	}}
	_, err := ts.getReport()
	require.Error(t, err)

	// Property without TextValue should succeed
	prop2 := Property{Name: "property2", Value: "value"}
	ts2 := Testsuites{Name: "cfv", Tests: 1, Testsuites: []Testsuite{
		{Name: "cfv", Errors: 0, Properties: &[]Property{prop2}},
	}}
	_, err = ts2.getReport()
	require.NoError(t, err)

	// Testcase with bad property should fail
	tc := Testcase{Name: "tc", ClassName: "cfv", Properties: &[]Property{prop1}}
	ts3 := Testsuites{Name: "cfv", Tests: 1, Testsuites: []Testsuite{
		{Name: "cfv", Errors: 0, Testcases: &[]Testcase{tc}},
	}}
	_, err = ts3.getReport()
	require.Error(t, err)
}

// --- SARIF reporter tests ---

func Test_sarifReport(t *testing.T) {
	reports := []Report{validReport, invalidReport, backslashReport}
	output := captureStdout(t, func() {
		err := (&SARIFReporter{}).Print(reports)
		require.NoError(t, err)
	})

	var log SARIFLog
	require.NoError(t, json.Unmarshal([]byte(output), &log))
	assert.Equal(t, SARIFVersion, log.Version)
	assert.Equal(t, SARIFSchema, log.Schema)
	require.Len(t, log.Runs, 1)
	assert.Equal(t, DriverName, log.Runs[0].Tool.Driver.Name)
	assert.Equal(t, DriverInfoURI, log.Runs[0].Tool.Driver.InfoURI)
	assert.Equal(t, DriverVersion, log.Runs[0].Tool.Driver.Version)

	results := log.Runs[0].Results
	require.Len(t, results, 3)
	assert.Equal(t, "pass", results[0].Kind)
	assert.Equal(t, "none", results[0].Level)
	assert.Contains(t, results[0].Locations[0].PhysicalLocation.ArtifactLocation.URI, "/fake/path/good.xml")

	assert.Equal(t, "fail", results[1].Kind)
	assert.Equal(t, "error", results[1].Level)
	assert.Equal(t, "unable to parse bad.xml file", results[1].Message.Text)

	// Backslash path converted
	assert.Contains(t, results[2].Locations[0].PhysicalLocation.ArtifactLocation.URI, "/fake/path/good.xml")
}

func Test_sarifReportWithRegion(t *testing.T) {
	reportWithPos := Report{
		FileName:         "bad.json",
		FilePath:         "/fake/path/bad.json",
		IsValid:          false,
		ValidationError:  errors.New("error at line 3 column 10"),
		ValidationErrors: []string{"error at line 3 column 10"},
		StartLine:        3,
		StartColumn:      10,
	}
	reportLineOnly := Report{
		FileName:         "bad.yaml",
		FilePath:         "/fake/path/bad.yaml",
		IsValid:          false,
		ValidationError:  errors.New("yaml: line 5: mapping error"),
		ValidationErrors: []string{"yaml: line 5: mapping error"},
		StartLine:        5,
	}

	var buf bytes.Buffer
	log, err := createSARIFReport([]Report{reportWithPos, reportLineOnly, validReport})
	require.NoError(t, err)

	sarifBytes, err := json.MarshalIndent(log, "", "  ")
	require.NoError(t, err)
	buf.Write(sarifBytes)

	output := buf.String()
	// reportWithPos should have region with startLine and startColumn
	assert.Contains(t, output, `"startLine": 3`)
	assert.Contains(t, output, `"startColumn": 10`)
	// reportLineOnly should have region with startLine only (no startColumn since it's 0)
	assert.Contains(t, output, `"startLine": 5`)
	// validReport should not have a region
	assert.NotContains(t, output, `"startLine": 0`)
}

func Test_sarifReportQuiet(t *testing.T) {
	output := captureStdout(t, func() {
		err := (&SARIFReporter{}).Print([]Report{quietReport})
		require.NoError(t, err)
	})
	assert.Empty(t, output)
}

func Test_sarifReportToFile(t *testing.T) {
	tmpDir := t.TempDir()
	err := NewSARIFReporter(tmpDir).Print([]Report{validReport})
	require.NoError(t, err)

	data, err := os.ReadFile(tmpDir + "/result.sarif")
	require.NoError(t, err)

	var log SARIFLog
	require.NoError(t, json.Unmarshal(data, &log))
	assert.Equal(t, SARIFVersion, log.Version)
	require.Len(t, log.Runs, 1)
	require.Len(t, log.Runs[0].Results, 1)
	assert.Equal(t, "pass", log.Runs[0].Results[0].Kind)
	assert.Contains(t, log.Runs[0].Results[0].Locations[0].PhysicalLocation.ArtifactLocation.URI, "/fake/path/good.xml")
}

func Test_sarifReportEmptyReports(t *testing.T) {
	log, err := createSARIFReport([]Report{})
	require.NoError(t, err)
	assert.Equal(t, SARIFVersion, log.Version)
	assert.Equal(t, SARIFSchema, log.Schema)
	require.Len(t, log.Runs, 1)
	assert.Equal(t, DriverName, log.Runs[0].Tool.Driver.Name)
	assert.Empty(t, log.Runs[0].Results)
}

func Test_sarifReportMultiErrorPositions(t *testing.T) {
	report := Report{
		FilePath:         "/path/bad.json",
		IsValid:          false,
		ValidationErrors: []string{"error one", "error two", "error three"},
		ErrorLines:       []int{10, 20},
		ErrorColumns:     []int{5},
		StartLine:        1,
		StartColumn:      1,
	}
	log, err := createSARIFReport([]Report{report})
	require.NoError(t, err)

	results := log.Runs[0].Results
	require.Len(t, results, 3)

	// First error: uses ErrorLines[0]=10, ErrorColumns[0]=5
	assert.Equal(t, "error one", results[0].Message.Text)
	require.NotNil(t, results[0].Locations[0].PhysicalLocation.Region)
	assert.Equal(t, 10, results[0].Locations[0].PhysicalLocation.Region.StartLine)
	assert.Equal(t, 5, results[0].Locations[0].PhysicalLocation.Region.StartColumn)

	// Second error: ErrorLines[1]=20, no ErrorColumns[1] — falls back to StartColumn
	assert.Equal(t, "error two", results[1].Message.Text)
	require.NotNil(t, results[1].Locations[0].PhysicalLocation.Region)
	assert.Equal(t, 20, results[1].Locations[0].PhysicalLocation.Region.StartLine)
	assert.Equal(t, 1, results[1].Locations[0].PhysicalLocation.Region.StartColumn)

	// Third error: no ErrorLines entry — falls back to StartLine/StartColumn
	assert.Equal(t, "error three", results[2].Message.Text)
	require.NotNil(t, results[2].Locations[0].PhysicalLocation.Region)
	assert.Equal(t, 1, results[2].Locations[0].PhysicalLocation.Region.StartLine)
	assert.Equal(t, 1, results[2].Locations[0].PhysicalLocation.Region.StartColumn)
}

func Test_sarifReportNoLineInfo(t *testing.T) {
	report := Report{
		FilePath:         "/path/bad.json",
		IsValid:          false,
		ValidationErrors: []string{"generic error"},
	}
	log, err := createSARIFReport([]Report{report})
	require.NoError(t, err)

	results := log.Runs[0].Results
	require.Len(t, results, 1)
	assert.Equal(t, "fail", results[0].Kind)
	assert.Equal(t, "generic error", results[0].Message.Text)
	assert.Nil(t, results[0].Locations[0].PhysicalLocation.Region)
}

// --- GitHub reporter Print tests ---

func Test_githubReporterPrint(t *testing.T) {
	failReport := Report{
		FilePath:        "a.json",
		IsValid:         false,
		ValidationError: errors.New("bad syntax"),
		StartLine:       3,
	}

	t.Run("writes annotations to file", func(t *testing.T) {
		tmpDir := t.TempDir()
		err := NewGitHubReporter(tmpDir).Print([]Report{failReport})
		require.NoError(t, err)

		data, err := os.ReadFile(tmpDir + "/result.txt")
		require.NoError(t, err)
		content := string(data)
		assert.Contains(t, content, "::error")
		assert.Contains(t, content, "a.json")
		assert.Contains(t, content, "bad syntax")
	})

	t.Run("returns error for bad path", func(t *testing.T) {
		err := NewGitHubReporter("/nonexistent/path/output").Print([]Report{failReport})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create a file")
	})

	t.Run("prints annotations to stdout", func(t *testing.T) {
		output := captureStdout(t, func() {
			err := NewGitHubReporter("").Print([]Report{failReport})
			require.NoError(t, err)
		})
		assert.Contains(t, output, "::error")
		assert.Contains(t, output, "a.json")
		assert.Contains(t, output, "bad syntax")
	})

	t.Run("all-valid reports produce no stdout output", func(t *testing.T) {
		output := captureStdout(t, func() {
			err := NewGitHubReporter("").Print([]Report{validReport})
			require.NoError(t, err)
		})
		assert.Empty(t, output)
	})

	t.Run("quiet mode suppresses stdout", func(t *testing.T) {
		quietFailReport := Report{
			FilePath:        "a.json",
			IsValid:         false,
			ValidationError: errors.New("bad syntax"),
			IsQuiet:         true,
		}
		output := captureStdout(t, func() {
			err := NewGitHubReporter("").Print([]Report{quietFailReport})
			require.NoError(t, err)
		})
		assert.Empty(t, output)
	})
}

// --- Grouped stdout tests ---

func Test_stdoutGroupedReports(t *testing.T) {
	t.Run("single group", func(t *testing.T) {
		singleGroup := map[string][]Report{
			"xml": mixedReports,
		}
		output := captureStdout(t, func() {
			err := PrintSingleGroupStdout(singleGroup)
			require.NoError(t, err)
		})
		assert.Contains(t, output, "xml")
		assert.Contains(t, output, "/fake/path/good.xml")
		assert.Contains(t, output, "/fake/path/bad.xml")
		assert.Contains(t, output, "Summary:")
		assert.Contains(t, output, "Total Summary:")
	})

	t.Run("pass-fail groups suppress per-group summary", func(t *testing.T) {
		passfailGroup := map[string][]Report{
			"Passed": {validReport},
			"Failed": {invalidReport},
		}
		output := captureStdout(t, func() {
			err := PrintSingleGroupStdout(passfailGroup)
			require.NoError(t, err)
		})
		assert.Contains(t, output, "Total Summary:")
		lines := strings.Split(output, "\n")
		perGroupSummaryCount := 0
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "Summary:") {
				perGroupSummaryCount++
			}
		}
		assert.Equal(t, 0, perGroupSummaryCount)
	})

	t.Run("double group", func(t *testing.T) {
		doubleGroup := map[string]map[string][]Report{
			"xml": {"directory": mixedReports},
		}
		output := captureStdout(t, func() {
			err := PrintDoubleGroupStdout(doubleGroup)
			require.NoError(t, err)
		})
		assert.Contains(t, output, "xml")
		assert.Contains(t, output, "directory")
		assert.Contains(t, output, "/fake/path/good.xml")
		assert.Contains(t, output, "Total Summary:")
	})

	t.Run("triple group", func(t *testing.T) {
		tripleGroup := map[string]map[string]map[string][]Report{
			"xml": {"directory": {"pass-fail": mixedReports}},
		}
		output := captureStdout(t, func() {
			err := PrintTripleGroupStdout(tripleGroup)
			require.NoError(t, err)
		})
		assert.Contains(t, output, "xml")
		assert.Contains(t, output, "directory")
		assert.Contains(t, output, "pass-fail")
		assert.Contains(t, output, "Total Summary:")
	})
}

// --- Grouped JSON tests ---

func Test_jsonGroupedReports(t *testing.T) {
	t.Run("single group", func(t *testing.T) {
		singleGroup := map[string][]Report{
			"xml": mixedReports,
		}
		output := captureStdout(t, func() {
			err := PrintSingleGroupJSON(singleGroup)
			require.NoError(t, err)
		})
		var parsed groupReportJSON
		require.NoError(t, json.Unmarshal([]byte(output), &parsed))
		require.Contains(t, parsed.Files, "xml")
		assert.Len(t, parsed.Files["xml"], 3)
		assert.Equal(t, 1, parsed.TotalPassed)
		assert.Equal(t, 2, parsed.TotalFailed)
	})

	t.Run("double group", func(t *testing.T) {
		doubleGroup := map[string]map[string][]Report{
			"xml": {"directory": mixedReports},
		}
		output := captureStdout(t, func() {
			err := PrintDoubleGroupJSON(doubleGroup)
			require.NoError(t, err)
		})
		var parsed doubleGroupReportJSON
		require.NoError(t, json.Unmarshal([]byte(output), &parsed))
		require.Contains(t, parsed.Files, "xml")
		require.Contains(t, parsed.Files["xml"], "directory")
		assert.Len(t, parsed.Files["xml"]["directory"], 3)
		assert.Equal(t, 1, parsed.TotalPassed)
		assert.Equal(t, 2, parsed.TotalFailed)
	})

	t.Run("triple group", func(t *testing.T) {
		tripleGroup := map[string]map[string]map[string][]Report{
			"xml": {"directory": {"pass-fail": mixedReports}},
		}
		output := captureStdout(t, func() {
			err := PrintTripleGroupJSON(tripleGroup)
			require.NoError(t, err)
		})
		var parsed tripleGroupReportJSON
		require.NoError(t, json.Unmarshal([]byte(output), &parsed))
		require.Contains(t, parsed.Files, "xml")
		require.Contains(t, parsed.Files["xml"], "directory")
		require.Contains(t, parsed.Files["xml"]["directory"], "pass-fail")
		assert.Len(t, parsed.Files["xml"]["directory"]["pass-fail"], 3)
		assert.Equal(t, 1, parsed.TotalPassed)
		assert.Equal(t, 2, parsed.TotalFailed)
	})
}

// --- Reporter file output tests (shared pattern) ---

func Test_reporterFileOutput(t *testing.T) {
	report := Report{
		FileName: "good.json",
		FilePath: "/fake/path/good.json",
		IsValid:  true,
	}

	for _, tc := range []struct {
		name        string
		newReporter func(string) Reporter
		extension   string
		verify      func(t *testing.T, data []byte)
	}{
		{
			"json",
			func(d string) Reporter { return NewJSONReporter(d) },
			"json",
			func(t *testing.T, data []byte) {
				t.Helper()
				assert.Contains(t, string(data), `"status": "passed"`)
				assert.Contains(t, string(data), `"passed": 1`)
				assert.Contains(t, string(data), `"/fake/path/good.json"`)
			},
		},
		{
			"junit",
			func(d string) Reporter { return NewJunitReporter(d) },
			"xml",
			func(t *testing.T, data []byte) {
				t.Helper()
				assert.Contains(t, string(data), `<?xml version="1.0" encoding="UTF-8"?>`)
				assert.Contains(t, string(data), `config-file-validator`)
				assert.Contains(t, string(data), `/fake/path/good.json`)
			},
		},
		{
			"sarif",
			func(d string) Reporter { return NewSARIFReporter(d) },
			"sarif",
			func(t *testing.T, data []byte) {
				t.Helper()
				assert.Contains(t, string(data), `"version": "2.1.0"`)
				assert.Contains(t, string(data), `"kind": "pass"`)
				assert.Contains(t, string(data), `/fake/path/good.json`)
			},
		},
	} {
		t.Run(tc.name+" to dir", func(t *testing.T) {
			tmpDir := t.TempDir()
			err := tc.newReporter(tmpDir).Print([]Report{report})
			require.NoError(t, err)

			actual, err := os.ReadFile(tmpDir + "/result." + tc.extension)
			require.NoError(t, err)
			tc.verify(t, actual)
		})

		t.Run(tc.name+" to file", func(t *testing.T) {
			tmpDir := t.TempDir()
			outPath := tmpDir + "/validator_result." + tc.extension
			err := tc.newReporter(outPath).Print([]Report{report})
			require.NoError(t, err)

			actual, err := os.ReadFile(outPath)
			require.NoError(t, err)
			tc.verify(t, actual)
		})

		t.Run(tc.name+" to stdout", func(t *testing.T) {
			err := tc.newReporter("").Print([]Report{report})
			require.NoError(t, err)
		})

		t.Run(tc.name+" to bad path", func(t *testing.T) {
			err := tc.newReporter("/nonexistent/path/output").Print([]Report{report})
			require.Error(t, err)
			assert.Regexp(t, "failed to create a file", err.Error())
		})
	}
}

// --- checkGroupsForPassFail ---

func Test_checkGroupsForPassFail(t *testing.T) {
	require.True(t, checkGroupsForPassFail("xml", "directory"))
	require.False(t, checkGroupsForPassFail("Passed"))
	require.False(t, checkGroupsForPassFail("xml", "Failed"))
}
