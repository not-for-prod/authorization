package entity

import "time"

// BindRegisteredUser records account creation and prepares normal completion.
// The application persists the account and this transition atomically.
func (t *Transaction) BindRegisteredUser(userID UserID, decision Decision, now time.Time) error {
	if t.Intent != IntentSignUp || t.VerifiedIdentity == nil || t.UserID != nil || userID == "" || !decision.Satisfied {
		return ErrInvalidTransition
	}
	next := t.Copy()
	before := next.eventSnapshot()
	next.UserID = &userID
	next.VerifiedIdentity = nil
	next.recordEvent("UserRegistered", &before, now)
	if err := next.PrepareCompletion(decision, now); err != nil {
		return err
	}
	next.events = append(t.events, next.events...)
	*t = next
	return nil
}
