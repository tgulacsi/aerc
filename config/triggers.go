package config

import (
	"errors"
	"fmt"

	"github.com/google/shlex"

	"git.sr.ht/~sircmpwn/aerc/lib/format"
	"git.sr.ht/~sircmpwn/aerc/models"
)

func (trig *TriggersConfig) ExecTrigger(triggerCmd string,
	triggerFmt func(string) (string, error)) error {

	if len(triggerCmd) == 0 {
		return errors.New("Trigger command empty")
	}
	triggerCmdParts, err := shlex.Split(triggerCmd)
	if err != nil {
		return err
	}

	command := make([]string, 0, len(triggerCmdParts))
	for _, part := range triggerCmdParts {
		formattedPart, err := triggerFmt(part)
		if err != nil {
			return err
		}
		command = append(command, formattedPart)
	}
	return trig.ExecuteCommand(command)
}

func (trig *TriggersConfig) ExecNewEmail(account *AccountConfig,
	conf *AercConfig, msg *models.MessageInfo) {
	err := trig.ExecTrigger(trig.NewEmail,
		func(part string) (string, error) {
			formatstr, args, err := format.ParseMessageFormat(
				part, conf.Ui.TimestampFormat,
				format.Ctx{
					FromAddress: account.From,
					AccountName: account.Name,
					MsgInfo:     msg},
			)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf(formatstr, args...), nil
		})
	if err != nil {
		fmt.Printf("Error from the new-email trigger: %s\n", err)
	}
}
