package imap

import (
	"git.sr.ht/~sircmpwn/aerc/worker/types"
)

func (imapw *IMAPWorker) handleRemoveDirectory(msg *types.RemoveDirectory) {
	if err := imapw.client.Delete(msg.Directory); err != nil {
		if msg.Quiet {
			return
		}
		imapw.worker.PostMessage(&types.Error{
			Message: types.RespondTo(msg),
			Error:   err,
		}, nil)
	} else {
		imapw.worker.PostMessage(&types.Done{Message: types.RespondTo(msg)}, nil)
	}
}
