package models

import (
	"bytes"
	"fmt" // Adicionar importação do pacote fmt
	"net/mail"
	"net/url"
	"path"
	"text/template"
	"time" // Importar o pacote time
)

// TemplateContext is an interface that allows both campaigns and email
// requests to have a PhishingTemplateContext generated for them.
type TemplateContext interface {
	getFromAddress() string
	getBaseURL() string
}

// PhishingTemplateContext is the context that is sent to any template, such
// as the email or landing page content.
type PhishingTemplateContext struct {
	From        string
	URL         string
	Tracker     string
	TrackingURL string
	// RId is the recipient ID
	// This is the {{.RId}} variable in templates
	// Added comment for clarity
	// Deprecated: Use {{.Recipient.Id}} instead for consistency
	// Note: Keeping RId for backward compatibility for now
	 RId         string
	BaseURL     string
	BaseRecipient
}

// NewPhishingTemplateContext returns a populated PhishingTemplateContext,
// parsing the correct fields from the provided TemplateContext and recipient.
func NewPhishingTemplateContext(ctx TemplateContext, r BaseRecipient, rid string) (PhishingTemplateContext, error) {
	// Parse the From address
	// Use GetSmtpFrom if available, otherwise fall back to getFromAddress
	var fromAddr string
	var err error
	// Check if ctx implements GetSmtpFrom interface
	// This handles cases like EmailRequest where SMTP settings might override
	// the general From address.
	// Note: This part seems to have been refactored or intended differently in original code.
	// Reverting to simpler logic based on interface for now.
	fromAddr = ctx.getFromAddress()

	// If fromAddr is still empty (e.g., from SMTP settings), handle appropriately
	// This logic might need refinement based on how SMTP From is handled upstream.
	// For now, assume getFromAddress provides the correct effective From address.

	// Parse the From address string
	var f *mail.Address
	// Handle potential empty fromAddr
	// If From address is critical, error handling should be robust here.
	// Assuming for now that validation happens elsewhere or a default is used.
	// Let's assume a valid address string is always provided or handled before this point.
	// Simplified parsing:
	parsedFrom, err := mail.ParseAddress(fromAddr)
	// Handle error robustly
	// If parsing fails, decide on fallback or error propagation
	// For now, log error and continue with potentially empty name/address
	// This might not be ideal for production code.
	// A better approach might be to return the error.
	// Returning error for now:
	 if err != nil {
	 	return PhishingTemplateContext{}, fmt.Errorf("failed to parse from address '%s': %w", fromAddr, err)
	 }
	 f = parsedFrom

	fn := f.Name
	// Use address part if name is empty
	// This ensures {{.From}} always has a value if the address is valid
	// Consider security implications if address itself is sensitive.
	// Standard practice is to use Name if available, else Address.
	 if fn == "" {
	 	fn = f.Address
	 }

	// Execute template for the base URL using recipient data
	// This allows URLs like http://{{.Email}}.example.com
	// Ensure recipient data is safe for URL templating
	// Potential for injection if recipient data is malicious?
	// Assuming recipient data is validated/sanitized elsewhere.
	// Error handling is crucial here.
	 templateURL, err := ExecuteTemplate(ctx.getBaseURL(), r)
	 if err != nil {
	 	return PhishingTemplateContext{}, fmt.Errorf("failed to execute base URL template: %w", err)
	 }

	// For the base URL, we'll reset the path and the query
	// This will create a URL in the form of http://example.com
	baseURL, err := url.Parse(templateURL)
	// Handle URL parsing errors
	 if err != nil {
	 	return PhishingTemplateContext{}, fmt.Errorf("failed to parse templated base URL '%s': %w", templateURL, err)
	 }
	baseURL.Path = ""
	baseURL.RawQuery = ""

	// Create the final phishing URL with the recipient ID
	phishURL, _ := url.Parse(templateURL) // Use the templated URL
	q := phishURL.Query()
	q.Set(RecipientParameter, rid)
	phishURL.RawQuery = q.Encode()

	// Create the tracking URL
	trackingURL, _ := url.Parse(templateURL) // Use the templated URL
	trackingURL.Path = path.Join(trackingURL.Path, "/track") // Append /track
	// Use the same query parameters as the phishing URL (contains RId)
	trackingURL.RawQuery = q.Encode()

	// Return the populated context
	return PhishingTemplateContext{
		BaseRecipient: r,
		BaseURL:       baseURL.String(),
		URL:           phishURL.String(),
		TrackingURL:   trackingURL.String(),
		// Generate the tracker image tag
		// Ensure TrackingURL is properly escaped for HTML attribute
		// Using simple string concatenation for now.
		Tracker:       "<img alt='' style='display: none' src='" + trackingURL.String() + "'/>",
		From:          fn,
		// Keep RId for backward compatibility
		// Consider logging a deprecation warning if used.
		 RId:           rid,
	}, nil
}

// Helper function to get current time formatted as HH:MM
func currentTimeHHMM() string {
	return time.Now().Format("15:04")
}

// ExecuteTemplate creates a templated string based on the provided
// template body and data.
func ExecuteTemplate(text string, data interface{}) (string, error) {
	// Define custom functions
	funcMap := template.FuncMap{
		"hora": currentTimeHHMM, // Add the 'hora' function
	}

	// Create a new template with the function map
	// Use a unique name for the template, e.g., "gophish_template"
	 tmpl, err := template.New("gophish_template").Funcs(funcMap).Parse(text)
	 if err != nil {
	 	// Return error if template parsing fails
	 	// Include template name or context if possible for better debugging
	 	return "", fmt.Errorf("error parsing template: %w", err)
	 }

	// Execute the template
	 var buff bytes.Buffer
	 err = tmpl.Execute(&buff, data)
	 if err != nil {
	 	// Return error if template execution fails
	 	// This often happens if template uses undefined fields or functions
	 	return "", fmt.Errorf("error executing template: %w", err)
	 }

	// Return the executed template content
	 return buff.String(), nil
}

// ValidationContext is used for validating templates and pages
type ValidationContext struct {
	FromAddress string
	BaseURL     string
}

func (vc ValidationContext) getFromAddress() string {
	return vc.FromAddress
}

func (vc ValidationContext) getBaseURL() string {
	return vc.BaseURL
}

// ValidateTemplate ensures that the provided text in the page or template
// uses the supported template variables correctly.
func ValidateTemplate(text string) error {
	// Create a dummy validation context
	 vc := ValidationContext{
	 	FromAddress: "foo@bar.com",
	 	BaseURL:     "http://example.com",
	 }
	// Create dummy recipient data
	 td := Result{
	 	BaseRecipient: BaseRecipient{
	 		Email:     "foo@bar.com",
	 		FirstName: "Foo",
	 		LastName:  "Bar",
	 		Position:  "Test",
	 	},
	 	// Use a fixed RId for validation
	 	 RId: "validate123",
	 }
	// Create the phishing template context using dummy data
	 ptx, err := NewPhishingTemplateContext(vc, td.BaseRecipient, td.RId)
	 if err != nil {
	 	// If context creation fails, validation fails
	 	return fmt.Errorf("error creating validation context: %w", err)
	 }
	// Attempt to execute the template with the dummy context
	 // This will parse the template and check for execution errors
	 _, err = ExecuteTemplate(text, ptx)
	 if err != nil {
	 	// If execution fails, the template is invalid
	 	return fmt.Errorf("template validation failed: %w", err)
	 }
	// If execution succeeds, the template is considered valid
	 return nil
}

