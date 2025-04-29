package models

import (
	"errors"
	"fmt"
	"math/rand"
	"net/mail"
	// "strings" // Removido - não utilizado
	"time"

	log "github.com/gophish/gophish/logger"
	"github.com/jinzhu/gorm"
)

// HeaderProfile represents a set of predefined email headers.
type HeaderProfile struct {
	Name    string
	Headers map[string]string
}

// PredefinedHeaderProfiles stores the known header profiles.
// Using constants for simplicity. Could be moved to config.json for more flexibility.
var PredefinedHeaderProfiles = map[string]HeaderProfile{
	"default": {
		Name:    "Default (Gophish)",
		Headers: map[string]string{ // Gophish default adds X-Mailer later
			"MIME-Version": "1.0",
		},
	},
	"apple_mail": {
		Name: "Apple Mail (macOS)",
		Headers: map[string]string{
			"MIME-Version": "1.0 (Mac OS X Mail 11.5 (3445.9.1))", // Example version
			"X-Mailer":     "Apple Mail (2.3445.9.1)",             // Example version
			// Content-Type with boundary is handled dynamically
		},
	},
	"outlook": {
		Name: "Microsoft Outlook (Desktop)",
		Headers: map[string]string{
			"MIME-Version":       "1.0",
			"X-Mailer":           "Microsoft Outlook 16.0", // Example version
			"Content-Language":   "en-us",
			"x-ms-has-attach":    "", // Common, value often empty or "yes"
			"x-ms-tnef-correlator": "", // Common, value often empty
			// Thread-Index is complex to generate accurately, omitting for now
			// Content-Type with boundary is handled dynamically
		},
	},
	"gmail_web": {
		Name: "Gmail (Web Interface)",
		Headers: map[string]string{
			"MIME-Version": "1.0",
			// No specific X-Mailer for web interface
			// Content-Type with boundary is handled dynamically
		},
	},
	"yahoo_web": {
		Name: "Yahoo Mail (Web Interface)",
		Headers: map[string]string{
			"MIME-Version": "1.0",
			// No specific X-Mailer for web interface
			// Content-Type with boundary is handled dynamically
		},
	},
}

// Template models hold the attributes for an email template to be sent to targets
type Template struct {
	Id             int64        `json:"id" gorm:"column:id; primary_key:yes"`
	UserId         int64        `json:"-" gorm:"column:user_id"`
	Name           string       `json:"name"`
	EnvelopeSender string       `json:"envelope_sender"`
	Subject        string       `json:"subject"`
	Text           string       `json:"text"`
	HTML           string       `json:"html" gorm:"column:html"`
	ModifiedDate   time.Time    `json:"modified_date"`
	Attachments    []Attachment `json:"attachments"`
	HeaderProfile  string       `json:"header_profile" gorm:"column:header_profile; default:\'default\'"` // Added field for header profile selection
}

// ErrTemplateNameNotSpecified is thrown when a template name is not specified
var ErrTemplateNameNotSpecified = errors.New("Template name not specified")

// ErrTemplateMissingParameter is thrown when a needed parameter is not provided
var ErrTemplateMissingParameter = errors.New("Need to specify at least plaintext or HTML content")

// ErrInvalidHeaderProfile is thrown when an unknown header profile is specified
var ErrInvalidHeaderProfile = errors.New("Invalid header profile specified")

// generateBoundary creates a random MIME boundary string.
func generateBoundary() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	const length = 30
	seededRand := rand.New(rand.NewSource(time.Now().UnixNano()))

	boundary := make([]byte, length)
	for i := range boundary {
		boundary[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(boundary)
}

// GetHeadersForProfile returns the headers for a given profile name, handling dynamic values.
func GetHeadersForProfile(profileName string) (map[string]string, string, error) {
	profile, ok := PredefinedHeaderProfiles[profileName]
	// Default to "default" profile if not found or empty
	 if !ok || profileName == "" {
	 	profileName = "default"
	 	profile = PredefinedHeaderProfiles[profileName]
	 }

	// Copy headers to avoid modifying the original map
	 headers := make(map[string]string)
	 for k, v := range profile.Headers {
	 	 headers[k] = v
	 }

	// Generate dynamic boundary for Content-Type
	 boundary := "----=_NextPart_" + generateBoundary()
	 if profileName == "apple_mail" {
	 	 boundary = "Apple-Mail=_" + generateBoundary()
	 }
	 // Add Content-Type header with dynamic boundary
	 headers["Content-Type"] = fmt.Sprintf("multipart/alternative; boundary=\"%s\"", boundary)

	return headers, boundary, nil
}

// Validate checks the given template to make sure values are appropriate and complete
func (t *Template) Validate() error {
	// Ensure HeaderProfile is valid if provided
	 if t.HeaderProfile != "" {
	 	 _, ok := PredefinedHeaderProfiles[t.HeaderProfile]
	 	 if !ok {
	 	 	 return ErrInvalidHeaderProfile
	 	 }
	 }

	 switch {
	 case t.Name == "":
	 	 return ErrTemplateNameNotSpecified
	 case t.Text == "" && t.HTML == "":
	 	 return ErrTemplateMissingParameter
	 case t.EnvelopeSender != "":
	 	 _, err := mail.ParseAddress(t.EnvelopeSender)
	 	 if err != nil {
	 	 	 return err
	 	 }
	 }
	 if err := ValidateTemplate(t.HTML); err != nil {
	 	 return err
	 }
	 if err := ValidateTemplate(t.Text); err != nil {
	 	 return err
	 }
	 for _, a := range t.Attachments {
	 	 if err := a.Validate(); err != nil {
	 	 	 return err
	 	 }
	 }

	 return nil
}

// GetTemplates returns the templates owned by the given user.
func GetTemplates(uid int64) ([]Template, error) {
	 ts := []Template{}
	 err := db.Where("user_id=?", uid).Find(&ts).Error
	 if err != nil {
	 	 log.Error(err)
	 	 return ts, err
	 }
	 for i := range ts {
	 	 // Get Attachments
	 	 err = db.Where("template_id=?", ts[i].Id).Find(&ts[i].Attachments).Error
	 	 if err == nil && len(ts[i].Attachments) == 0 {
	 	 	 ts[i].Attachments = make([]Attachment, 0)
	 	 }
	 	 if err != nil && err != gorm.ErrRecordNotFound {
	 	 	 log.Error(err)
	 	 	 return ts, err
	 	 }
	 	 // Ensure HeaderProfile has a default value if empty (for older records)
	 	 if ts[i].HeaderProfile == "" {
	 	 	 ts[i].HeaderProfile = "default"
	 	 }
	 }
	 return ts, err
}

// GetTemplate returns the template, if it exists, specified by the given id and user_id.
func GetTemplate(id int64, uid int64) (Template, error) {
	 t := Template{}
	 err := db.Where("user_id=? and id=?", uid, id).Find(&t).Error
	 if err != nil {
	 	 log.Error(err)
	 	 return t, err
	 }

	 // Get Attachments
	 err = db.Where("template_id=?", t.Id).Find(&t.Attachments).Error
	 if err != nil && err != gorm.ErrRecordNotFound {
	 	 log.Error(err)
	 	 return t, err
	 }
	 if err == nil && len(t.Attachments) == 0 {
	 	 t.Attachments = make([]Attachment, 0)
	 }
	 // Ensure HeaderProfile has a default value if empty (for older records)
	 if t.HeaderProfile == "" {
	 	 t.HeaderProfile = "default"
	 }
	 return t, err
}

// GetTemplateByName returns the template, if it exists, specified by the given name and user_id.
func GetTemplateByName(n string, uid int64) (Template, error) {
	 t := Template{}
	 err := db.Where("user_id=? and name=?", uid, n).Find(&t).Error
	 if err != nil {
	 	 log.Error(err)
	 	 return t, err
	 }

	 // Get Attachments
	 err = db.Where("template_id=?", t.Id).Find(&t.Attachments).Error
	 if err != nil && err != gorm.ErrRecordNotFound {
	 	 log.Error(err)
	 	 return t, err
	 }
	 if err == nil && len(t.Attachments) == 0 {
	 	 t.Attachments = make([]Attachment, 0)
	 }
	 // Ensure HeaderProfile has a default value if empty (for older records)
	 if t.HeaderProfile == "" {
	 	 t.HeaderProfile = "default"
	 }
	 return t, err
}

// PostTemplate creates a new template in the database.
func PostTemplate(t *Template) error {
	 // Set default profile if empty
	 if t.HeaderProfile == "" {
	 	 t.HeaderProfile = "default"
	 }
	 // Insert into the DB
	 if err := t.Validate(); err != nil {
	 	 return err
	 }
	 err := db.Save(t).Error
	 if err != nil {
	 	 log.Error(err)
	 	 return err
	 }

	 // Save every attachment
	 for i := range t.Attachments {
	 	 t.Attachments[i].TemplateId = t.Id
	 	 err := db.Save(&t.Attachments[i]).Error
	 	 if err != nil {
	 	 	 log.Error(err)
	 	 	 return err
	 	 }
	 }
	 return nil
}

// PutTemplate edits an existing template in the database.
// Per the PUT Method RFC, it presumes all data for a template is provided.
func PutTemplate(t *Template) error {
	 // Set default profile if empty
	 if t.HeaderProfile == "" {
	 	 t.HeaderProfile = "default"
	 }
	 if err := t.Validate(); err != nil {
	 	 return err
	 }
	 // Delete all attachments, and replace with new ones
	 err := db.Where("template_id=?", t.Id).Delete(&Attachment{}).Error
	 if err != nil && err != gorm.ErrRecordNotFound {
	 	 log.Error(err)
	 	 return err
	 }
	 if err == gorm.ErrRecordNotFound {
	 	 err = nil
	 }
	 for i := range t.Attachments {
	 	 t.Attachments[i].TemplateId = t.Id
	 	 err := db.Save(&t.Attachments[i]).Error
	 	 if err != nil {
	 	 	 log.Error(err)
	 	 	 return err
	 	 }
	 }

	 // Save final template
	 // Use Updates to only update provided fields, or Save to replace all
	 // Save seems more appropriate for PUT, but ensure all fields are sent from frontend
	 // Using Save as per original logic
	 err = db.Save(t).Error // Save will update all fields based on primary key
	 if err != nil {
	 	 log.Error(err)
	 	 return err
	 }
	 return nil
}

// DeleteTemplate deletes an existing template in the database.
// An error is returned if a template with the given user id and template id is not found.
func DeleteTemplate(id int64, uid int64) error {
	 // Delete attachments
	 err := db.Where("template_id=?", id).Delete(&Attachment{}).Error
	 // GORM V1 might return RecordNotFound, V2 might not. Check behavior.
	 // Assuming RecordNotFound is not a critical error for deletion.
	 if err != nil && err != gorm.ErrRecordNotFound {
	 	 log.Error(err)
	 	 return err
	 }

	 // Finally, delete the template itself
	 // Use primary key directly for deletion
	 err = db.Where("user_id=?", uid).Delete(&Template{Id: id}).Error
	 if err != nil {
	 	 log.Error(err)
	 	 return err
	 }
	 // Check if any rows were affected to confirm deletion
	 // This requires checking the result of the Delete operation, which might vary by GORM version.
	 // Skipping explicit check for now.
	 return nil
}

