package prompt

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfirmLabel(t *testing.T) {
	tests := []struct {
		name     string
		label    string
		head     string
		question string
	}{
		{
			name:     "single line",
			label:    "Do you really want to delete these templates?",
			head:     "",
			question: "Do you really want to delete these templates",
		},
		{
			name:     "leading lines are split off",
			label:    "⚠️  This makes them public\nDo you really want to publish these templates?",
			head:     "⚠️  This makes them public",
			question: "Do you really want to publish these templates",
		},
		{
			name:     "several leading lines are kept together",
			label:    "first\nsecond\nPublish?",
			head:     "first\nsecond",
			question: "Publish",
		},
		{
			name:     "trailing newline is ignored",
			label:    "Publish?\n",
			head:     "",
			question: "Publish",
		},
		{
			name:     "label without question mark is untouched",
			label:    "Publish",
			head:     "",
			question: "Publish",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			head, question := confirmLabel(test.label)
			assert.Equal(t, test.head, head)
			assert.Equal(t, test.question, question)
		})
	}
}
