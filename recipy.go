package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

func main() {
	filename := "tartiflette.recipy.hcl"
	src, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Fatalf("failed to open %s: %v", filename, err)
	}
	os.Exit(retMain(src, filename))
}

var preparationActionsSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "slice", LabelNames: []string{"ingredient"}},
		{Type: "boil", LabelNames: []string{"ingredient"}},
	},
}

var stackContentSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "add"},
	},
}

var stackSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "stack", LabelNames: []string{"name"}},
	},
}

type Action struct {
	Duration string `hcl:"during"`
	Verb     string
	What     string
}

func (a Action) String() string {
	s := fmt.Sprintf("%s %s", a.Verb, a.What)
	if a.Duration != "" {
		s += fmt.Sprintf(" during %s", a.Duration)
	}
	return s
}

func retMain(src []byte, filename string) int {
	file, diags := hclsyntax.ParseConfig(src, filename, hcl.Pos{Byte: 0, Line: 1, Column: 1})
	if diags.HasErrors() {
		log.Printf("%v", diags)
		return 1
	}
	files := map[string]*hcl.File{
		filename: file,
	}

	preparationContent, prepRest, diags := file.Body.PartialContent(preparationActionsSchema)
	if diags.HasErrors() {
		return writeDiags(files, diags)
	}

	var actions []Action
	for _, block := range preparationContent.Blocks {
		action := Action{
			Verb: block.Type,
			What: block.Labels[0],
		}
		switch block.Type {
		case "slice":
			// nothing to do here since this is empty
		case "boil":
			boilBody := block.Body
			diags := gohcl.DecodeBody(boilBody, nil, &action)
			if diags.HasErrors() {
				return writeDiags(files, diags)
			}
		}
		actions = append(actions, action)
	}

	stackContent, moreDiags := prepRest.Content(stackSchema)
	diags = append(diags, moreDiags...)
	if diags.HasErrors() {
		return writeDiags(files, diags)
	}

	var stack struct {
		What string
		In   string   `hcl:"in,optional"`
		Rest hcl.Body `hcl:",remain"`
	}
	for _, block := range stackContent.Blocks {
		switch block.Type {
		case "stack":
			stackBody := block.Body
			diags := gohcl.DecodeBody(stackBody, nil, &stack)
			if diags.HasErrors() {
				return writeDiags(files, diags)
			}
			stack.What = block.Labels[0]

			break // there can only be one stack here and we don't enforce it
		}
	}

	nestedStackContent, moreDiags := stack.Rest.Content(stackContentSchema)
	diags = append(diags, moreDiags...)
	if diags.HasErrors() {
		return writeDiags(files, diags)
	}
	stackAddsRequire := []string{}
	for _, block := range nestedStackContent.Blocks {
		switch block.Type {
		case "add":
			var stackSpec = &hcldec.ObjectSpec{
				"what":     &hcldec.AttrSpec{Name: "what", Type: cty.String, Required: true},
				"quantity": &hcldec.AttrSpec{Name: "quantity", Type: cty.String},
			}

			// tell what is required by this block :
			traversals := hcldec.Variables(block.Body, stackSpec)
			for _, traversal := range traversals {
				split := traversal.SimpleSplit()
				stackAddsRequire = append(stackAddsRequire, split.RootName())
			}
		}
	}

	fmt.Printf("To prepare a %s:\n\n", stack.What)
	for _, action := range actions {
		fmt.Printf("* %s\n", action)
	}
	fmt.Printf("\nThen stack in a %s\n\n", stack.In)

	fmt.Println("Then... you have to figure it out")

	return 0
}

func writeDiags(files map[string]*hcl.File, diags hcl.Diagnostics) int {
	// write HCL errors/diagnostics if any.
	b := bytes.NewBuffer(nil)
	err := hcl.NewDiagnosticTextWriter(b, files, 80, false).WriteDiagnostics(diags)
	if err != nil {
		log.Fatalf("could not write diagnostic: %v", err)
		return 1
	}
	if b.Len() != 0 {
		if diags.HasErrors() {
			log.Fatal(b.String())
			return 1
		}
		log.Print(b.String())
	}
	return 0
}
