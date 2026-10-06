package starlings

import (
	"strconv"
	"strings"
)

const cdnBase = "https://cdn.discordapp.com/"

// cdnURL builds a CDN link. Discord serves animated assets - whose hashes start
// with "a_" - as GIFs, and everything else as PNG. A size of zero omits the
// query parameter and takes Discord's default.
func cdnURL(path, hash string, size int) string {
	ext := ".png"
	if strings.HasPrefix(hash, "a_") {
		ext = ".gif"
	}
	url := cdnBase + path + ext
	if size > 0 {
		url += "?size=" + strconv.Itoa(size)
	}
	return url
}

// defaultAvatarURL returns the avatar Discord assigns to users who have not
// uploaded one. Migrated accounts (discriminator "0") are bucketed by their ID;
// legacy accounts by their discriminator.
func defaultAvatarURL(u *User) string {
	var index uint64
	if u.Discriminator == "" || u.Discriminator == "0" {
		index = (uint64(u.ID) >> 22) % 6
	} else {
		d, _ := strconv.ParseUint(u.Discriminator, 10, 64)
		index = d % 5
	}
	return cdnBase + "embed/avatars/" + strconv.FormatUint(index, 10) + ".png"
}
