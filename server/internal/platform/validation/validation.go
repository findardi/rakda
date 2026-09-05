package validation

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/findardi/rakda/server/internal/platform/response"
	"github.com/go-playground/validator/v10"
)

var validate *validator.Validate

func init() {
	validate = validator.New(validator.WithRequiredStructEnabled())

	validate.RegisterTagNameFunc(func(field reflect.StructField) string {
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name == "-" {
			return ""
		}
		return name
	})
}

// Bind decodes the JSON body into T and validates it, answering 400 itself.
// ok is false when the handler must return without doing anything else.
func Bind[T any](w http.ResponseWriter, r *http.Request) (req T, ok bool) {
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid body request", nil)
		return req, false
	}
	if errs := Validate(&req); errs != nil {
		response.Error(w, http.StatusBadRequest, "validation failed", errs)
		return req, false
	}
	return req, true
}

func Validate(s any) []response.FieldError {
	err := validate.Struct(s)
	if err == nil {
		return nil
	}

	var invalid *validator.InvalidValidationError
	if errors.Is(err, invalid) {
		return []response.FieldError{{
			Field:   "_",
			Message: err.Error(),
		}}
	}

	var verrs validator.ValidationErrors
	errors.As(err, &verrs)

	out := make([]response.FieldError, 0, len(verrs))
	for _, fe := range verrs {
		out = append(out, response.FieldError{
			Field:   fe.Field(),
			Message: messageFor(fe),
		})
	}

	return out
}

func messageFor(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "required"
	case "email":
		return "invalid email format"
	case "min":
		return fmt.Sprintf("minimal %s characters", fe.Param())
	case "max":
		return fmt.Sprintf("maximal %s characters", fe.Param())
	case "numeric":
		return "must be number"
	case "len":
		return fmt.Sprintf("must %s characters", fe.Param())
	case "eqfield":
		return fmt.Sprintf("must same with %s", fe.Param())
	case "required_without":
		return fmt.Sprintf("must fill if %s is empty", fe.Param())
	default:
		return "invalid"

	}
}
