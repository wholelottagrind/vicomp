package encode

// Preset is a named set of settings.
type Preset struct {
	Name        string
	Description string
	Settings    Settings
}

// Builtin presets, in display order.
var Builtin = []Preset{
	{
		Name:        "telegram",
		Description: "balanced: small file, crisp text, plays everywhere",
		Settings:    Settings{Codec: "h264", Encoder: "software", Quality: 26, Speed: "slow", MaxRes: 1080, MaxFPS: 30, Audio: "128k"},
	},
	{
		Name:        "tiny",
		Description: "as small as reasonable, for long recordings",
		Settings:    Settings{Codec: "h264", Encoder: "software", Quality: 30, Speed: "slow", MaxRes: 720, MaxFPS: 24, Audio: "64k"},
	},
	{
		Name:        "quality",
		Description: "looks like the original, still much smaller than the .mov",
		Settings:    Settings{Codec: "h264", Encoder: "software", Quality: 20, Speed: "slow", MaxRes: 0, MaxFPS: 0, Audio: "128k"},
	},
	{
		Name:        "fast",
		Description: "hardware encoder: done in seconds, file a bit bigger",
		Settings:    Settings{Codec: "h264", Encoder: "hardware", Quality: 26, Speed: "medium", MaxRes: 1080, MaxFPS: 30, Audio: "128k"},
	},
	{
		Name:        "classic",
		Description: "x264 defaults, like your old command (crf 23, medium, original size)",
		Settings:    Settings{Codec: "h264", Encoder: "software", Quality: 23, Speed: "medium", MaxRes: 0, MaxFPS: 0, Audio: "128k"},
	},
}
