package email

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitOTP_SixDigits(t *testing.T) {
	result := SplitOTP("840921")
	assert.Equal(t, []string{"8", "4", "0", "-", "9", "2", "1"}, result)
}

func TestSplitOTP_ShortInput(t *testing.T) {
	result := SplitOTP("123")
	assert.Equal(t, []string{"123"}, result)
}

func TestSplitOTP_EmptyInput(t *testing.T) {
	result := SplitOTP("")
	assert.Equal(t, []string{""}, result)
}

func TestSplitParagraphs_MultiParagraph(t *testing.T) {
	result := SplitParagraphs("First paragraph.\n\nSecond paragraph.")
	require.Len(t, result, 2)
	assert.Equal(t, "First paragraph.", result[0])
	assert.Equal(t, "Second paragraph.", result[1])
}

func TestSplitParagraphs_SingleNewlines(t *testing.T) {
	result := SplitParagraphs("Line one\nLine two")
	require.Len(t, result, 1)
	assert.Equal(t, "Line one Line two", result[0])
}

func TestSplitParagraphs_BlankLines(t *testing.T) {
	result := SplitParagraphs("\n\nFirst\n\n\n\nSecond\n\n")
	require.Len(t, result, 2)
	assert.Equal(t, "First", result[0])
	assert.Equal(t, "Second", result[1])
}

func TestNewRenderer_NonZeroYear(t *testing.T) {
	r := NewRenderer(BaseData{AppName: "GoApps", AppURL: "http://localhost"})
	assert.NotZero(t, r.base.Year)
}

func TestRenderer_UnknownTemplate_ReturnsError(t *testing.T) {
	r := NewRenderer(BaseData{AppName: "GoApps", AppURL: "http://localhost"})
	_, err := r.Render("nonexistent", nil)
	assert.Error(t, err)
}

func TestRenderer_RenderOTP(t *testing.T) {
	r := NewRenderer(BaseData{AppName: "GoApps", AppURL: "http://localhost:3000"})
	data := OTPData{
		BaseData:       r.BaseData(),
		RecipientEmail: "test@example.com",
		OTPDigits:      SplitOTP("840921"),
		ExpiryMinutes:  10,
		Purpose:        "password reset",
	}
	html, err := r.Render("otp", data)
	require.NoError(t, err)
	assert.Contains(t, html, "GoApps")
	assert.Contains(t, html, "password reset")
	assert.Contains(t, html, "Expires in 10 minutes")
	assert.Contains(t, html, ">8<") // first OTP digit in a td
	assert.Contains(t, html, ">9<") // first digit of second group
	assert.NotContains(t, html, "cdn.tailwindcss.com")
	assert.NotContains(t, html, "<script")
}

func TestRenderer_RenderSecurity(t *testing.T) {
	r := NewRenderer(BaseData{AppName: "GoApps", AppURL: "http://localhost:3000"})
	data := SecurityData{
		BaseData:      r.BaseData(),
		RecipientName: "Ilham Ramadhan",
		Feature:       "Two-Factor Authentication",
		Action:        "enabled",
		IPAddress:     "192.168.1.1",
		OccurredAt:    "June 11, 2026 at 14:30 WIB",
		SecureURL:     "http://localhost:3000/settings/security",
	}
	html, err := r.Render("security", data)
	require.NoError(t, err)
	assert.Contains(t, html, "Ilham Ramadhan")
	assert.Contains(t, html, "Two-Factor Authentication")
	assert.Contains(t, html, "enabled")
	assert.Contains(t, html, "192.168.1.1")
	assert.Contains(t, html, "Review Account")
}

func TestRenderer_RenderSecurity_NoIPNoUserAgent(t *testing.T) {
	r := NewRenderer(BaseData{AppName: "GoApps", AppURL: "http://localhost:3000"})
	data := SecurityData{
		BaseData:  r.BaseData(),
		Feature:   "Password",
		Action:    "reset",
		SecureURL: "http://localhost:3000/settings/security",
	}
	html, err := r.Render("security", data)
	require.NoError(t, err)
	assert.NotContains(t, html, "IP Address")
}

func TestRenderer_RenderWelcome(t *testing.T) {
	r := NewRenderer(BaseData{AppName: "GoApps", AppURL: "http://localhost:3000"})
	data := WelcomeData{
		BaseData:       r.BaseData(),
		RecipientName:  "Ilham Ramadhan",
		RecipientEmail: "ilham@mutugading.com",
		LoginURL:       "http://localhost:3000/login",
	}
	html, err := r.Render("welcome", data)
	require.NoError(t, err)
	assert.Contains(t, html, "Ilham Ramadhan")
	assert.Contains(t, html, "ilham@mutugading.com")
	assert.Contains(t, html, "http://localhost:3000/login")
}
