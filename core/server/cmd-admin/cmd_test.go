package cmdadmin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"charm.land/huh/v2"
	"github.com/cyril-fl/tide/config"
	"github.com/cyril-fl/tide/core/domain"
	"github.com/cyril-fl/tide/core/services/huhform"
	"github.com/cyril-fl/tide/core/services/postgrey"
	"github.com/cyril-fl/tide/core/services/process/adduser"
	"github.com/cyril-fl/tide/core/services/process/createschema"
	"github.com/cyril-fl/tide/core/services/process/createsession"
	"github.com/cyril-fl/tide/core/services/process/resetpassword"
	"github.com/spf13/cobra"
)

// The commands commit on postgrey.GetDb(), not in a rolled-back transaction: each account made here is deleted
// at the end.
func setup(t *testing.T) context.Context {
	t.Helper()
	ctx := context.Background()
	env := postgrey.NewEnvironment()
	env.ReadEnv()
	db := env.Connect()
	schema := createschema.Process(ctx, db)
	schema.Run()
	if schema.HasErr() {
		t.Fatalf("create schema: %v", schema.GetErr())
	}
	previous := postgrey.GetDb()
	postgrey.SetDb(db)
	t.Cleanup(func() {
		postgrey.SetDb(previous)
		db.Close()
	})
	return ctx
}

func account(t *testing.T, ctx context.Context) string {
	t.Helper()
	name := fmt.Sprintf("test-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		if _, err := postgrey.GetDb().NewDelete().Model((*domain.User)(nil)).Where("name = ?", name).Exec(ctx); err != nil {
			t.Errorf("delete %s: %v", name, err)
		}
	})
	return name
}

// keys types each field of the next huh forms, then waits: the form focuses the next field a key late, a key typed
// at once would land in the previous one.
func keys(t *testing.T, fields ...string) {
	t.Helper()
	reader, writer := io.Pipe()
	go func() {
		for _, field := range fields {
			if _, err := writer.Write([]byte(field)); err != nil {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()
	huhform.Input = reader
	t.Cleanup(func() {
		huhform.Input = nil
		writer.Close()
	})
}

// run calls a command as cobra does and returns what it printed. The password comes from stdin, or from the form
// once keys are given.
func run(t *testing.T, ctx context.Context, runE func(*cobra.Command, []string) error, stdin string, args ...string) (string, error) {
	t.Helper()
	command := &cobra.Command{}
	command.Flags().Bool("password-stdin", huhform.Input == nil, "")
	command.SetContext(ctx)

	in, inWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	inWriter.WriteString(stdin)
	inWriter.Close()
	out, outWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdinBefore, stdoutBefore := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = in, outWriter
	defer func() { os.Stdin, os.Stdout = stdinBefore, stdoutBefore }()

	runErr := runE(command, args)
	outWriter.Close()
	printed, _ := io.ReadAll(out)
	return string(printed), runErr
}

func logIn(t *testing.T, ctx context.Context, name, password string) error {
	t.Helper()
	process := createsession.Process(ctx, postgrey.GetDb(), createsession.Input{Name: name, Password: password})
	process.Run()
	return process.GetErr()
}

func TestAddUser(t *testing.T) {
	ctx := setup(t)
	name := account(t, ctx)

	printed, err := run(t, ctx, AddUser, "a password\n", name)
	if err != nil || printed != "Successfully created account "+name+"\n" {
		t.Fatalf("printed %q, err = %v; want the account created", printed, err)
	}
	if err := logIn(t, ctx, name, "a password"); err != nil {
		t.Errorf("log in: %v; want the password read from stdin, without its newline", err)
	}

	for label, test := range map[string]struct {
		stdin string
		want  error
	}{
		"name taken":     {"another password\n", adduser.ErrNameTaken},
		"short password": {"short\n", adduser.ErrShortPassword},
		"no password":    {"", io.EOF},
	} {
		printed, err := run(t, ctx, AddUser, test.stdin, name)
		if !errors.Is(err, test.want) || printed != "" {
			t.Errorf("%s: printed %q, err = %v; want %v", label, printed, err, test.want)
		}
	}
}

func TestResetPassword(t *testing.T) {
	ctx := setup(t)
	name := account(t, ctx)
	if _, err := run(t, ctx, AddUser, "the old password\n", name); err != nil {
		t.Fatal(err)
	}

	printed, err := run(t, ctx, ResetPassword, "the new password\r\n", name)
	if err != nil || !strings.HasPrefix(printed, "Successfully reset the password of "+name) {
		t.Fatalf("printed %q, err = %v; want the password reset", printed, err)
	}
	if err := logIn(t, ctx, name, "the new password"); err != nil {
		t.Errorf("log in with the new password: %v", err)
	}
	if err := logIn(t, ctx, name, "the old password"); !errors.Is(err, createsession.ErrBadCredentials) {
		t.Errorf("log in with the old password: err = %v, want %v", err, createsession.ErrBadCredentials)
	}

	if _, err := run(t, ctx, ResetPassword, "the new password\n", name+"-nobody"); !errors.Is(err, resetpassword.ErrNotFound) {
		t.Errorf("unknown account: err = %v, want %v", err, resetpassword.ErrNotFound)
	}
}

func TestAskPassword(t *testing.T) {
	ctx := setup(t)
	name := account(t, ctx)

	keys(t, "a password\r", "another one\r", "\x03")
	if _, err := run(t, ctx, AddUser, "", name); !errors.Is(err, huh.ErrUserAborted) {
		t.Fatalf("passwords that differ: err = %v, want the form kept until aborted", err)
	}
	keys(t, "a password\r", "a password\r")
	if printed, err := run(t, ctx, AddUser, "", name); err != nil || printed != "Successfully created account "+name+"\n" {
		t.Fatalf("printed %q, err = %v; want the account created", printed, err)
	}
	if err := logIn(t, ctx, name, "a password"); err != nil {
		t.Errorf("log in: %v; want the password typed twice", err)
	}
}

func TestRotateSecret(t *testing.T) {
	ctx := setup(t)
	t.Setenv(config.Env("OLD_SECRET"), "b2xkIHNlY3JldCBvZiB0aGUgc2VydmVyLCAzMiBieXQ=")

	printed, err := run(t, ctx, RotateSecret, "")
	if err != nil || !strings.HasPrefix(printed, "Successfully sealed ") {
		t.Errorf("printed %q, err = %v; want the keys sealed", printed, err)
	}

	// A key no secret opens stops the rotation.
	name := account(t, ctx)
	broken := domain.User{Name: name, PasswordHash: "-", DataKey: "bm90IGEga2V5"}
	if _, err := postgrey.GetDb().NewInsert().Model(&broken).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	if printed, err := run(t, ctx, RotateSecret, ""); err == nil || printed != "" {
		t.Errorf("printed %q, err = %v; want the broken key to stop the rotation", printed, err)
	}
}
