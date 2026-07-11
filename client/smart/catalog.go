/* SPDX-License-Identifier: MIT */

package smart

const AIServiceID = "ai"

type Service struct {
	ID             string
	Name           string
	Description    string
	ProcessNames   []string
	Domains        []string
	DomainSuffixes []string
}

var serviceCatalog = []Service{
	{
		ID:          "discord",
		Name:        "Discord",
		Description: "Приложение, звонки и Web",
		ProcessNames: []string{
			"Discord.exe", "DiscordCanary.exe", "DiscordPTB.exe",
		},
		Domains: []string{"discord.com", "discord.gg", "dis.gd"},
		DomainSuffixes: []string{
			".discord.com", ".discord.gg", ".discordapp.com", ".discordapp.net",
			".discordcdn.com", ".discord.media", ".discordstatus.com", ".dis.gd",
		},
	},
	{
		ID:          "youtube",
		Name:        "YouTube",
		Description: "Видео, обложки и служебные API",
		Domains:     []string{"youtube.com", "youtu.be", "youtube-nocookie.com"},
		DomainSuffixes: []string{
			".youtube.com", ".youtu.be", ".youtube-nocookie.com", ".googlevideo.com",
			".ytimg.com", ".youtubei.googleapis.com", ".youtube.googleapis.com", ".ggpht.com",
		},
	},
	{
		ID:          "telegram",
		Name:        "Telegram",
		Description: "Desktop, Web и звонки",
		ProcessNames: []string{
			"Telegram.exe",
		},
		Domains: []string{"t.me", "telegram.org", "telegram.me", "telegram.dog"},
		DomainSuffixes: []string{
			".t.me", ".telegram.org", ".telegram.me", ".telegram.dog", ".tdesktop.com",
		},
	},
	{
		ID:          "instagram",
		Name:        "Instagram",
		Description: "Сайт, медиа и CDN",
		Domains:     []string{"instagram.com"},
		DomainSuffixes: []string{
			".instagram.com", ".cdninstagram.com", ".fbcdn.net",
		},
	},
	{
		ID:          AIServiceID,
		Name:        "Нейросети",
		Description: "ChatGPT, Claude, Gemini, Grok и другие",
		ProcessNames: []string{
			"ChatGPT.exe", "Claude.exe", "Perplexity.exe", "Copilot.exe",
			"Cursor.exe", "Windsurf.exe",
		},
		Domains: []string{
			"chatgpt.com", "openai.com", "claude.ai", "anthropic.com",
			"gemini.google.com", "aistudio.google.com", "ai.google.dev",
			"generativelanguage.googleapis.com", "notebooklm.google.com",
			"grok.com", "x.ai", "perplexity.ai", "pplx.ai",
			"deepseek.com", "deepseek.ai", "copilot.microsoft.com",
			"copilot.com", "mistral.ai", "poe.com", "character.ai",
			"meta.ai", "huggingface.co", "midjourney.com", "suno.com",
			"runwayml.com", "leonardo.ai", "ideogram.ai", "cursor.com",
			"windsurf.com", "codeium.com", "phind.com", "kimi.com",
			"qwen.ai", "manus.im", "genspark.ai", "stability.ai",
			"dreamstudio.ai", "replicate.com", "fal.ai", "together.ai",
			"groq.com", "cohere.com", "openrouter.ai",
		},
		DomainSuffixes: []string{
			".chatgpt.com", ".openai.com", ".oaistatic.com", ".oaiusercontent.com",
			".oaistatsig.com", ".openaimerge.com", ".workos.com", ".workoscdn.com",
			".claude.ai", ".anthropic.com", ".claudeusercontent.com",
			".gemini.google.com", ".aistudio.google.com", ".ai.google.dev",
			".generativelanguage.googleapis.com", ".notebooklm.google.com", ".bard.google.com",
			".grok.com", ".x.ai", ".perplexity.ai", ".pplx.ai",
			".deepseek.com", ".deepseek.ai", ".copilot.microsoft.com", ".copilot.com",
			".githubcopilot.com", ".individual.githubcopilot.com",
			".business.githubcopilot.com", ".enterprise.githubcopilot.com",
			".copilot-proxy.githubusercontent.com", ".origin-tracker.githubusercontent.com",
			".copilot-telemetry.githubusercontent.com",
			".cloud.microsoft", ".mistral.ai", ".poe.com", ".character.ai",
			".characterai.io", ".meta.ai", ".huggingface.co", ".hf.co", ".hf.space",
			".midjourney.com", ".midjourneycdn.com", ".suno.com", ".suno.ai",
			".runwayml.com", ".leonardo.ai", ".ideogram.ai", ".cursor.com", ".cursor.sh",
			".windsurf.com", ".codeium.com", ".phind.com", ".kimi.com",
			".moonshot.ai", ".moonshot.cn", ".qwen.ai", ".dashscope.aliyuncs.com",
			".manus.im", ".genspark.ai", ".stability.ai", ".dreamstudio.ai",
			".replicate.com", ".fal.ai", ".together.ai", ".groq.com", ".cohere.com",
			".openrouter.ai",
		},
	},
}

func copyStrings(values []string) []string {
	return append([]string(nil), values...)
}

func ServiceCatalog() []Service {
	out := make([]Service, len(serviceCatalog))
	for i, service := range serviceCatalog {
		out[i] = service
		out[i].ProcessNames = copyStrings(service.ProcessNames)
		out[i].Domains = copyStrings(service.Domains)
		out[i].DomainSuffixes = copyStrings(service.DomainSuffixes)
	}
	return out
}

func DefaultServiceIDs() []string {
	ids := make([]string, 0, len(serviceCatalog))
	for _, service := range serviceCatalog {
		ids = append(ids, service.ID)
	}
	return ids
}

func selectedServices(ids []string) []Service {
	selected := make(map[string]bool, len(ids))
	for _, id := range ids {
		selected[id] = true
	}
	services := make([]Service, 0, len(ids))
	for _, service := range serviceCatalog {
		if selected[service.ID] {
			services = append(services, service)
		}
	}
	return services
}
