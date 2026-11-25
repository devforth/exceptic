package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"time"

	"github.com/devforth/exceptic/internal/notify"
	"github.com/devforth/exceptic/internal/platform/config"
	"github.com/devforth/exceptic/internal/storage/store"
)

const verificationCodeLifetime = 15 * time.Minute

// ResendCooldown is the minimum time between two codes to one address; a
// variable for tests.
var ResendCooldown = 30 * time.Second

// Verification mails and checks the codes that confirm email addresses;
// a confirmation belongs to the address (a destination), not a connection.
type Verification struct {
	logger      *slog.Logger
	store       store.Store
	factory     notify.Factory
	connections *Connections
	now         func() time.Time
}

// newVerification: connections finds an address that is already connected.
func newVerification(logger *slog.Logger, database store.Store, factory notify.Factory, connections *Connections) *Verification {
	return &Verification{logger: logger, store: database, factory: factory, connections: connections, now: time.Now}
}

// StartEmail takes an entered address: one already connected is refused
// (Duplicate, not named outside access), a confirmed one is accepted at once, otherwise a code is
// mailed. An error is a failure of the database.
func (verification *Verification) StartEmail(ctx context.Context, access store.Access, entered string) (EmailStep, error) {
	address, ok := config.ParseEmail(entered)

	if !ok {
		return EmailStep{State: EmailInput, Address: entered, Error: "Enter a valid email address."}, nil
	}

	// One connection per address: edit the existing one instead.
	existing, err := verification.connections.findDuplicate(ctx, access, config.ChannelEmail, address, "")

	if err != nil || existing != nil {
		return EmailStep{State: EmailInput, Address: address, Duplicate: existing}, err
	}

	destination, err := verification.DestinationFor(ctx, address)

	if err != nil {
		return EmailStep{}, err
	}

	if destination.Verified {
		step := verifiedStep(destination)
		step.Notice = "Confirmed earlier: no code needed."

		return step, nil
	}

	return verification.sendCode(ctx, destination)
}

// verifiedStep is the step of a confirmed address.
func verifiedStep(destination store.Destination) EmailStep {
	return EmailStep{State: EmailVerified, Address: destination.Address, VerifiedAt: destination.VerifiedAt}
}

// FindDestination is the destination of an address entered before; ok is
// false when it was never entered.
func (verification *Verification) FindDestination(ctx context.Context, address string) (destination store.Destination, ok bool, err error) {
	destination, err = verification.store.FindEmailDestination(ctx, strings.ToLower(strings.TrimSpace(address)))

	if errors.Is(err, store.ErrNotFound) {
		return destination, false, nil
	}

	return destination, err == nil, err
}

// againStep asks for the address again: the destination id is unknown.
var againStep = EmailStep{State: EmailInput, Error: "Enter the email address again."}

// ConfirmCode checks a code entered for the address of destination id; an
// address confirmed already (a repeated submit) stays confirmed.
func (verification *Verification) ConfirmCode(ctx context.Context, id string, code string) (EmailStep, error) {
	destination, err := verification.EmailDestination(ctx, id)

	if err != nil {
		return againStep, nil
	}

	if destination.Verified {
		return verifiedStep(destination), nil
	}

	return verification.confirm(ctx, destination, strings.TrimSpace(code))
}

// DestinationFor returns the email destination of address (lowercase),
// creating it unconfirmed the first time the address is entered.
func (verification *Verification) DestinationFor(ctx context.Context, address string) (store.Destination, error) {
	destination, err := verification.store.FindEmailDestination(ctx, address)

	if !errors.Is(err, store.ErrNotFound) {
		return destination, err
	}

	id, err := verification.store.CreateDestination(ctx, store.Destination{Type: config.ChannelEmail, Address: address}, verification.now())

	if err != nil {
		return store.Destination{}, err
	}

	return verification.store.GetDestination(ctx, id)
}

// ErrNotEmail: the destination id a form sent is not an email destination.
var ErrNotEmail = errors.New("not an email destination")

// EmailDestination reads the email destination with id; ErrNotEmail when it
// is a chat, store.ErrNotFound when it does not exist.
func (verification *Verification) EmailDestination(ctx context.Context, id string) (store.Destination, error) {
	return emailDestination(ctx, verification.store, id)
}

func emailDestination(ctx context.Context, database store.Store, id string) (store.Destination, error) {
	destination, err := database.GetDestination(ctx, id)

	if err == nil && destination.Type != config.ChannelEmail {
		return destination, ErrNotEmail
	}

	return destination, err
}

// sendCode mails a new code unless one was sent within ResendCooldown, and
// returns the step to show. An error is a failure of the database.
func (verification *Verification) sendCode(ctx context.Context, destination store.Destination) (EmailStep, error) {
	step := EmailStep{State: EmailCode, Address: destination.Address}

	if wait := destination.VerifySentAt.Add(ResendCooldown).Sub(verification.now()); wait > 0 {
		step.Notice = fmt.Sprintf("A code was sent less than %s ago; check the inbox or send again in %ds.", ResendCooldown, int(wait.Seconds())+1)

		return step, nil
	}

	code := newVerificationCode()
	now := verification.now()

	if err := verification.store.SetVerificationCode(ctx, destination.ID, codeHash(destination.ID, code), now.Add(verificationCodeLifetime), now); err != nil {
		return step, err
	}

	sendCtx, cancel := context.WithTimeout(ctx, 30*time.Second)

	defer cancel()

	email := verification.factory.Email(destination.Address)

	if err := email.SendVerificationCode(sendCtx, code, verificationCodeLifetime); err != nil {
		verification.logger.Error("send verification code", "destination_id", destination.ID, "err", err)

		step.Error, step.Unsent = "The email could not be sent: "+err.Error(), true
	} else {
		step.Notice = fmt.Sprintf("We sent a %d-digit code to %s. It is valid for %s.",
			config.VerificationCodeDigits, destination.Address, config.HumanDuration(verificationCodeLifetime))
	}

	return step, nil
}

// confirm checks a typed code and returns the step to show. An error is a
// failure of the database.
func (verification *Verification) confirm(ctx context.Context, destination store.Destination, code string) (EmailStep, error) {
	step := EmailStep{State: EmailCode, Address: destination.Address}

	result, err := verification.store.CheckVerificationCode(ctx, destination.ID, codeHash(destination.ID, code), verification.now())

	if err != nil {
		return step, err
	}

	switch result {
	case store.VerifyOK:
		step.State, step.VerifiedAt, step.Notice = EmailVerified, verification.now(), "Address confirmed."
	case store.VerifyExpired:
		step.Error = "The code has expired. Send a new one."
	case store.VerifyTooManyTries:
		step.Error = "Too many wrong codes. Send a new one."
	default:
		step.Error = "Wrong code."
	}

	return step, nil
}

// codeHash is what is stored instead of the code: tied to the destination,
// so a code cannot confirm another address.
func codeHash(destinationID string, code string) string {
	sum := sha256.Sum256(fmt.Appendf(nil, "%s:%s", destinationID, code))

	return hex.EncodeToString(sum[:])
}

func newVerificationCode() string {
	limit := new(big.Int).Exp(big.NewInt(10), big.NewInt(config.VerificationCodeDigits), nil)
	number, _ := rand.Int(rand.Reader, limit)

	return fmt.Sprintf("%0*d", config.VerificationCodeDigits, number.Int64())
}
