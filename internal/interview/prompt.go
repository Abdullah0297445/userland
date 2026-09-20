package interview

import (
	"errors"

	"github.com/charmbracelet/huh"
)

type option struct {
	label    string
	value    string
	selected bool
}

func run(field huh.Field) error {
	err := huh.NewForm(huh.NewGroup(field)).Run()
	if errors.Is(err, huh.ErrUserAborted) {
		return ErrAborted
	}
	return err
}

func selectOne(title, description string, options []option) (string, error) {
	var value string
	choices := make([]huh.Option[string], 0, len(options))
	for _, o := range options {
		choices = append(choices, huh.NewOption(o.label, o.value))
	}
	err := run(huh.NewSelect[string]().Title(title).Description(description).Options(choices...).Value(&value))
	return value, err
}

func selectMany(title, description string, options []option) ([]string, error) {
	var values []string
	choices := make([]huh.Option[string], 0, len(options))
	for _, o := range options {
		choices = append(choices, huh.NewOption(o.label, o.value).Selected(o.selected))
	}
	err := run(huh.NewMultiSelect[string]().Title(title).Description(description).Options(choices...).Value(&values))
	return values, err
}

func input(title string, hidden bool, validate func(string) error) (string, error) {
	var value string
	field := huh.NewInput().Title(title).Validate(validate).Value(&value)
	if hidden {
		field.EchoMode(huh.EchoModePassword)
	}
	if err := run(field); err != nil {
		return "", err
	}
	if err := validate(value); err != nil {
		return "", err
	}
	return value, nil
}

func Confirm(title, description string) (bool, error) {
	var yes bool
	err := run(huh.NewConfirm().Title(title).Description(description).Affirmative("Yes").Negative("No").Value(&yes))
	return yes, err
}
