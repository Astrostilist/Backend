package alisa

// AstroProfile contains the user's basic astrological data,
// which is inserted into the prompt template.
type AstroProfile struct {
	UserID     string
	BirthDate  string
	BirthTime  string
	BirthPlace string
}
