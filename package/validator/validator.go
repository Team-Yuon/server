package validator

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func GetValidationErrors(err error) []ValidationError {
	var errors []ValidationError

	if validationErrors, ok := err.(validator.ValidationErrors); ok {
		for _, e := range validationErrors {
			errors = append(errors, ValidationError{
				Field:   getFieldName(e),
				Message: getErrorMessage(e),
			})
		}
	}

	return errors
}

func getFieldName(e validator.FieldError) string {
	field := e.Field()
	return strings.ToLower(field[:1]) + field[1:]
}

func getErrorMessage(e validator.FieldError) string {
	switch e.Tag() {
	case "required":
		return fmt.Sprintf("%s는 필수 항목입니다", e.Field())
	case "email":
		return "유효한 이메일 주소를 입력하세요"
	case "min":
		return fmt.Sprintf("최소 %s 이상이어야 합니다", e.Param())
	case "max":
		return fmt.Sprintf("최대 %s 이하여야 합니다", e.Param())
	case "len":
		return fmt.Sprintf("길이가 %s이어야 합니다", e.Param())
	case "url":
		return "유효한 URL을 입력하세요"
	case "oneof":
		return fmt.Sprintf("다음 값 중 하나여야 합니다: %s", e.Param())
	default:
		return fmt.Sprintf("%s 검증에 실패했습니다", e.Field())
	}
}

func Init() {
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		// 커스텀 검증 규칙 등록 가능
		_ = v
	}
}
