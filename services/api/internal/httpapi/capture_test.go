package httpapi

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/crabnuggets/folio/services/api/internal/domain"
)

func TestCaptureRejectsInvalidModelAmountsAndSections(t *testing.T) {
	sheet, err := domain.NewState().GetMonthlySheet("2026-10")
	if err != nil {
		t.Fatal(err)
	}
	section := sheet.Sections[0].ID
	valid := `{"message":"Review these items.","items":[{"name":" Salary ","section_id":"` + section + `","amount_minor":"9007199254740993","note":"October salary"}]}`
	result, err := parseCaptureResponse(valid, sheet)
	if err != nil || len(result.Items) != 1 || result.Items[0].Name != "Salary" || result.Items[0].AmountMinor == nil || *result.Items[0].AmountMinor != domain.Money(9007199254740993) {
		t.Fatalf("exact large amount was not preserved: %#v, %v", result, err)
	}
	for name, text := range map[string]string{
		"number amount":   strings.Replace(valid, `"9007199254740993"`, `1234`, 1),
		"negative":        strings.Replace(valid, `9007199254740993`, `-1`, 1),
		"out of range":    strings.Replace(valid, `9007199254740993`, `9223372036854775808`, 1),
		"decimal":         strings.Replace(valid, `9007199254740993`, `12.34`, 1),
		"noncanonical":    strings.Replace(valid, `9007199254740993`, `001`, 1),
		"missing amount":  strings.Replace(valid, `"amount_minor":"9007199254740993",`, ``, 1),
		"unknown section": strings.Replace(valid, section, "unknown", 1),
		"unknown field":   strings.Replace(valid, `"note":`, `"execute":true,"note":`, 1),
		"missing message": strings.Replace(valid, `"message":"Review these items.",`, ``, 1),
		"trailing object": valid + `{}`,
		"null items":      `{"message":"Question","items":null}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseCaptureResponse(text, sheet); err == nil {
				t.Fatal("invalid model output was accepted")
			}
		})
	}
	for _, text := range []string{`{"message":"Which amount?","items":[]}`, strings.Replace(valid, `"9007199254740993"`, `null`, 1)} {
		if _, err := parseCaptureResponse(text, sheet); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCaptureValidatesAttachmentBeforeSending(t *testing.T) {
	file := func(media, data string) *captureAttachment {
		return &captureAttachment{Name: "receipt", MediaType: media, Data: base64.StdEncoding.EncodeToString([]byte(data))}
	}
	for name, in := range map[string]captureInput{
		"empty":         {},
		"oversize text": {Text: strings.Repeat("x", 12001)},
		"bad base64":    {Attachment: &captureAttachment{Name: "receipt.png", MediaType: "image/png", Data: "!?"}},
		"mismatch":      {Attachment: file("image/png", "%PDF-1.7")},
		"unsupported":   {Attachment: file("image/svg+xml", "<svg></svg>")},
		"binary text":   {Attachment: file("text/plain", "abc\x00def")},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := captureContent(in); err == nil {
				t.Fatal("invalid capture was accepted")
			}
		})
	}
	for media, data := range map[string]string{
		"image/png":       "\x89PNG\r\n\x1a\nreceipt",
		"image/jpeg":      "\xff\xd8\xffreceipt",
		"image/webp":      "RIFF0000WEBPreceipt",
		"application/pdf": "%PDF-1.7\nreceipt",
		"text/plain":      "Salary SGD 1234.56",
	} {
		t.Run(media, func(t *testing.T) {
			content, err := captureContent(captureInput{Text: "Add this", Attachment: file(media, data)})
			if err != nil || len(content) != 2 {
				t.Fatalf("valid capture failed: %#v, %v", content, err)
			}
		})
	}
}
