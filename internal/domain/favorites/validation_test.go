package favorites

import (
	"strings"
	"testing"
)

func TestValidateAndNormalizeInput(t *testing.T) {
	tests := []struct {
		name    string
		input   FavoriteInput
		wantErr error
	}{
		{
			name: "valid input",
			input: FavoriteInput{
				CardName:   "Charizard",
				SetName:    "Base Set",
				CardNumber: "4",
			},
			wantErr: nil,
		},
		{
			name: "empty card name",
			input: FavoriteInput{
				CardName: "",
				SetName:  "Base Set",
			},
			wantErr: ErrCardNameRequired,
		},
		{
			name: "whitespace card name",
			input: FavoriteInput{
				CardName: "   ",
				SetName:  "Base Set",
			},
			wantErr: ErrCardNameRequired,
		},
		{
			name: "empty set name",
			input: FavoriteInput{
				CardName: "Charizard",
				SetName:  "",
			},
			wantErr: ErrSetNameRequired,
		},
		{
			name: "whitespace set name",
			input: FavoriteInput{
				CardName: "Charizard",
				SetName:  "   ",
			},
			wantErr: ErrSetNameRequired,
		},
		{
			name: "card name too long",
			input: FavoriteInput{
				CardName: strings.Repeat("a", MaxCardNameLength+1),
				SetName:  "Base Set",
			},
			wantErr: ErrCardNameTooLong,
		},
		{
			name: "set name too long",
			input: FavoriteInput{
				CardName: "Charizard",
				SetName:  strings.Repeat("a", MaxSetNameLength+1),
			},
			wantErr: ErrSetNameTooLong,
		},
		{
			name: "card number too long",
			input: FavoriteInput{
				CardName:   "Charizard",
				SetName:    "Base Set",
				CardNumber: strings.Repeat("1", MaxCardNumberLength+1),
			},
			wantErr: ErrCardNumberTooLong,
		},
		{
			name: "image url too long",
			input: FavoriteInput{
				CardName: "Charizard",
				SetName:  "Base Set",
				ImageURL: strings.Repeat("h", MaxImageURLLength+1),
			},
			wantErr: ErrImageURLTooLong,
		},
		{
			name: "notes too long",
			input: FavoriteInput{
				CardName: "Charizard",
				SetName:  "Base Set",
				Notes:    strings.Repeat("n", MaxNotesLength+1),
			},
			wantErr: ErrNotesTooLong,
		},
		{
			name: "max length card name is valid",
			input: FavoriteInput{
				CardName: strings.Repeat("a", MaxCardNameLength),
				SetName:  "Base Set",
			},
			wantErr: nil,
		},
		{
			name: "valid with all optional fields",
			input: FavoriteInput{
				CardName:   "Charizard",
				SetName:    "Base Set",
				CardNumber: "4",
				ImageURL:   "https://example.com/image.png",
				Notes:      "My favorite card!",
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := tt.input // Make a copy to pass as pointer
			err := ValidateAndNormalizeInput(&input)
			if err != tt.wantErr {
				t.Errorf("ValidateAndNormalizeInput() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateAndNormalizeInput_TrimAssignsBack(t *testing.T) {
	input := FavoriteInput{
		CardName:   "  Charizard  ",
		SetName:    "  Base Set  ",
		CardNumber: "4",
	}

	err := ValidateAndNormalizeInput(&input)
	if err != nil {
		t.Fatalf("ValidateAndNormalizeInput() unexpected error = %v", err)
	}

	if input.CardName != "Charizard" {
		t.Errorf("ValidateAndNormalizeInput() CardName = %q, want %q", input.CardName, "Charizard")
	}
	if input.SetName != "Base Set" {
		t.Errorf("ValidateAndNormalizeInput() SetName = %q, want %q", input.SetName, "Base Set")
	}
}

func TestIsValidationError(t *testing.T) {
	validationErrors := []error{
		ErrCardNameRequired,
		ErrSetNameRequired,
		ErrCardNameTooLong,
		ErrSetNameTooLong,
		ErrCardNumberTooLong,
		ErrImageURLTooLong,
		ErrNotesTooLong,
	}

	for _, err := range validationErrors {
		if !IsValidationError(err) {
			t.Errorf("IsValidationError(%v) = false, want true", err)
		}
	}

	// Non-validation errors
	if IsValidationError(ErrFavoriteNotFound) {
		t.Error("IsValidationError(ErrFavoriteNotFound) = true, want false")
	}
	if IsValidationError(ErrFavoriteAlreadyExists) {
		t.Error("IsValidationError(ErrFavoriteAlreadyExists) = true, want false")
	}
	if IsValidationError(nil) {
		t.Error("IsValidationError(nil) = true, want false")
	}
}
