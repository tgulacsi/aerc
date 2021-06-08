package lib

import (
	"strings"

	"git.sr.ht/~sircmpwn/aerc/models"
)

func FindPlaintext(bs *models.BodyStructure, path []int) []int {
	for i, part := range bs.Parts {
		cur := append(path, i+1)
		if strings.ToLower(part.MIMEType) == "text" &&
			strings.ToLower(part.MIMESubType) == "plain" {
			return cur
		}
		if strings.ToLower(part.MIMEType) == "multipart" {
			if path := FindPlaintext(part, cur); path != nil {
				return path
			}
		}
	}
	return nil
}

func FindFirstNonMultipart(bs *models.BodyStructure, path []int) []int {
	for i, part := range bs.Parts {
		cur := append(path, i+1)
		mimetype := strings.ToLower(part.MIMEType)
		if mimetype != "multipart" {
			return cur
		}
		if path := FindFirstNonMultipart(part, cur); path != nil {
			return path
		}
	}
	return nil
}
