package tui

import (
	"hash/fnv"

	"github.com/charmbracelet/lipgloss"
)

// AvatarSize controls how avatars are rendered in the UI.
type AvatarSize int

const (
	AvatarOff   AvatarSize = iota // no avatar
	AvatarSmall                   // single-line pea: (o.o)
	AvatarLarge                   // two-line pea: hat + face
)

// Avatar is a small pea-inspired character built from composable parts.
// Each member gets a deterministic combination based on their name hash.
type Avatar struct {
	Hat   string         // single char/string above the pea (empty = none)
	Eyes  Eyes           // left + right eye characters
	Mouth string         // single char between the eyes
	Color lipgloss.Color // pea body tint (matches name colour)
}

// Eyes are the left+right characters inside the pea face.
type Eyes struct {
	Left  string
	Right string
}

// ---------- parts library ----------

var hatParts = []string{
	"",   // none
	"_",  // cap
	"^",  // party hat
	"~",  // beanie
	"*",  // star
	"♔",  // crown
	"¬",  // beret
	".",  // antenna dot
}

var eyeParts = []Eyes{
	{Left: "o", Right: "o"},   // round
	{Left: "●", Right: "●"},   // dot
	{Left: "^", Right: "^"},   // happy
	{Left: "■", Right: "■"},   // glasses
	{Left: "*", Right: "*"},   // star
	{Left: "~", Right: "o"},   // wink-L
	{Left: "o", Right: "~"},   // wink-R
	{Left: "°", Right: "°"},   // wide
	{Left: "-", Right: "-"},   // chill
	{Left: "'", Right: "'"},   // sleepy
}

var mouthParts = []string{
	"‿", // smile
	".", // neutral
	"u", // grin
	"_", // flat
	"o", // surprised
	"v", // smirk
	"~", // wavy
	"w", // cat
}

// ---------- deterministic selection ----------

// AvatarFor returns a deterministic avatar for the given name.
func AvatarFor(name string) Avatar {
	if name == "" || name == "human" || name == "me" {
		return humanAvatar()
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	sum := h.Sum32()

	hatIdx := int(sum) % len(hatParts)
	eyeIdx := int(sum/uint32(len(hatParts))) % len(eyeParts)
	mouthIdx := int(sum/uint32(len(hatParts)*len(eyeParts))) % len(mouthParts)

	return Avatar{
		Hat:   hatParts[hatIdx],
		Eyes:  eyeParts[eyeIdx],
		Mouth: mouthParts[mouthIdx],
		Color: colorFor(name),
	}
}

func humanAvatar() Avatar {
	return Avatar{
		Hat:   "",
		Eyes:  Eyes{Left: "o", Right: "o"},
		Mouth: "‿",
		Color: humanColor,
	}
}

// ---------- rendering ----------

// RenderSmall returns a single-line coloured pea: (o.o)
func (a Avatar) RenderSmall() string {
	face := "(" + a.Eyes.Left + a.Mouth + a.Eyes.Right + ")"
	return lipgloss.NewStyle().Foreground(a.Color).Render(face)
}

// RenderLarge returns a two-line coloured pea with hat above face.
// If the pea has no hat, falls back to single-line.
//
//	 ^
//	(o.o)
func (a Avatar) RenderLarge() string {
	style := lipgloss.NewStyle().Foreground(a.Color)
	face := "(" + a.Eyes.Left + a.Mouth + a.Eyes.Right + ")"
	if a.Hat == "" {
		return style.Render(face)
	}
	return style.Render(" "+a.Hat) + "\n" + style.Render(face)
}

// Render returns the avatar at the given size, or empty string for AvatarOff.
func (a Avatar) Render(size AvatarSize) string {
	switch size {
	case AvatarLarge:
		return a.RenderLarge()
	case AvatarSmall:
		return a.RenderSmall()
	default:
		return ""
	}
}
