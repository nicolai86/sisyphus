package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"
)

func Test_UpdateGemfile_Rewrite(t *testing.T) {
	f, err := os.Open("./fakes/outdated.log")
	if err != nil {
		t.Fatal(err)
	}
	output := ParseLog(f)

	f2, err := os.Open("./fakes/Gemfile")
	if err != nil {
		t.Fatal(err)
	}

	var b = bytes.Buffer{}
	io.Copy(&b, f2)

	var b2 = bytes.Buffer{}
	UpdateGemfile(output, &b, &b2)
	fmt.Printf("%#v\n\n", b2.String())
}

func Test_ParseLog_DetectUpdates(t *testing.T) {
	expectations := []string{
		"rvm-capistrano",
		"mechanize",
		"test-unit",
		"gmaps4rails",
		"turbolinks",
		"strong_parameters",
		"prawn",
		"mysql2",
		"sass-rails",
		"bootstrap-sass",
		"font-awesome-rails",
		"jquery-rails",
		"globalize",
		"annotate",
		"workflow",
		"rubyzip",
		"devise",
		"acts_as_list",
		"Ascii85",
		"axlsx",
	}

	expectedVersionFrom := map[string]string{
		"rvm-capistrano":     "1.2.0",
		"mechanize":          "2.7.2",
		"test-unit":          "3.0",
		"gmaps4rails":        "2.1",
		"turbolinks":         "2.3",
		"strong_parameters":  "0.1.6",
		"prawn":              "0.12.0",
		"mysql2":             "0.4",
		"sass-rails":         "3.2.3",
		"bootstrap-sass":     "3.1.1",
		"font-awesome-rails": "3.1.1.1",
		"jquery-rails":       "2.1.4",
		"globalize":          "3.0.0",
		"annotate":           "2.4.0",
		"workflow":           "0.8.1",
		"rubyzip":            "0.9.4",
		"devise":             "2.2.4",
		"acts_as_list":       "0.4.0",
		"Ascii85":            "1.0.1",
		"axlsx":              "1.2",
	}

	expectedVersionTo := map[string]string{
		"rvm-capistrano":     "1.5.6",
		"mechanize":          "2.7.4",
		"test-unit":          "3.2.1",
		"gmaps4rails":        "2.1.2",
		"turbolinks":         "5.0.1",
		"strong_parameters":  "0.2.3",
		"prawn":              "2.1.0",
		"mysql2":             "0.4.4",
		"sass-rails":         "5.0.6",
		"bootstrap-sass":     "3.3.7",
		"font-awesome-rails": "4.6.3.1",
		"jquery-rails":       "4.1.1",
		"globalize":          "5.0.1",
		"annotate":           "2.7.1",
		"workflow":           "1.2.0",
		"rubyzip":            "1.2.0",
		"devise":             "4.2.0",
		"acts_as_list":       "0.7.6",
		"Ascii85":            "1.0.2",
		"axlsx":              "2.0.1",
	}

	f, err := os.Open("./fakes/outdated.log")
	if err != nil {
		t.Fatal(err)
	}

	output := ParseLog(f)
	for _, expectation := range expectations {
		if _, ok := output.Updates[expectation]; !ok {
			t.Fatalf("Expected %q to be present in updates, but wasn't", expectation)
		}
	}

	for dep, wanted := range expectedVersionFrom {
		if output.Updates[dep].Wanted != wanted {
			t.Fatalf("Expected %q to be locked at %q, but was %q", dep, wanted, output.Updates[dep].Wanted)
		}
	}

	for dep, latest := range expectedVersionTo {
		if output.Updates[dep].Latest != latest {
			t.Fatalf("Expected %q to be suggested to %q, but was %q", dep, latest, output.Updates[dep].Latest)
		}
	}
}
