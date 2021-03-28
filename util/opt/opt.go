package opt

import "fmt"

// String holds an optional string value.
type String struct {
	Value *string
}

// NullString represents an empty optional String.
var NullString = String{nil}

// NewString creates a new optional String with the given value.
func NewString(value string) String {
	return String{&value}
}

// NewString creates a new optional String with the given pointer.
// When nil, the String is considered null, but an empty String is valid.
func NewStringWithPtr(value *string) String {
	return String{value}
}

// NewNotEmptyString creates a new optional String with the given value or
// returns NullString if the value is an empty string.
func NewNotEmptyString(value string) String {
	if value == "" {
		return NullString
	} else {
		return NewString(value)
	}
}

// IsNull returns whether the optional String has no value.
func (s String) IsNull() bool {
	return s.Value == nil
}

// IsEmpty returns whether the optional String has an empty string for value.
func (s String) IsEmpty() bool {
	return !s.IsNull() && *s.Value == ""
}

// NonEmpty returns a null String if the String is empty.
func (s String) NonEmpty() String {
	if s.IsEmpty() {
		return NullString
	} else {
		return s
	}
}

// Or returns the receiver if it is not null, otherwise the given optional
// String.
func (s String) Or(other String) String {
	if s.IsNull() {
		return other
	} else {
		return s
	}
}

// OrString returns the optional String value or the given default string if
// it is null.
func (s String) OrString(alt string) String {
	if s.IsNull() {
		return NewString(alt)
	} else {
		return s
	}
}

// Unwrap returns the optional String value or an empty String if none is set.
func (s String) Unwrap() string {
	if s.IsNull() {
		return ""
	} else {
		return *s.Value
	}
}

func (s String) Equal(other String) bool {
	return s.Value == other.Value ||
		(s.Value != nil && other.Value != nil && *s.Value == *other.Value)
}

func (s String) String() string {
	return s.Unwrap()
}

func (s String) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%v"`, s)), nil
}
