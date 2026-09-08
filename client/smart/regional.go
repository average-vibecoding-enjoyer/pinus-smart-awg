package smart

import "strings"

const VKServiceID = "vk"
const RussianServiceID = "russian-services"
const RegionalCatalogRevision = "2026-09-08"

// These groups are bundled with the client independently of the signed
// legacy catalog. Never route a whole TLD, ASN, shared cloud or browser process.
var regionalServices = []Service{
	{ID: VKServiceID, Name: "VK", Description: "Сайт, фото, музыка, видео и звонки",
		ProcessNames: []string{"vk-calls.exe", "vk-messenger.exe", "vk-messenger-x64.exe", "VKMessenger.exe"},
		// Production hosts found in calls_landing and its official TLS certificates.
		Domains: []string{"api.ok.ru", "api.mycdn.me", "st.mycdn.me", "vkcalls.production.vklanding.ru", "vkcalls-native-ac.vk-apps.com", "upload.object2.vk-apps.com"},
		DomainSuffixes: []string{
			".vk.ru", ".vk.com", ".vk.me", ".vkvideo.ru", ".vkvideo.com", ".vk.cc",
			".vkuserphoto.ru", ".vkuserdocs.ru", ".userapi.com",
			".vkuseraudio.net", ".vkuseraudio.ru", ".vkuseraudio.com",
			".vkuservideo.net", ".vkuservideo.ru", ".vkuservideo.com", ".vkuserlive.net",
			".vk-cdn.net", ".vk-portal.net", ".vkclips.app", ".vkcombo.ru", ".vkpay.com",
			".calls.okcdn.ru", ".calls-sdk.cdn-vk.ru",
		}},
	{ID: RussianServiceID, Name: "Белые списки", Description: "Популярные российские сервисы",
		DomainSuffixes: []string{
			".ya.ru", ".yandex.ru", ".yandex.com", ".yandex.net", ".yastatic.net", ".kinopoisk.ru",
			".mail.ru", ".imgsmail.ru", ".ok.ru", ".okcdn.ru", ".dzen.ru", ".dzeninfra.ru", ".max.ru",
			".ozon.ru", ".ozon.com", ".ozone.ru", ".ozonusercontent.com",
			".wildberries.ru", ".wb.ru", ".wbbasket.ru", ".wbstatic.net", ".wbcontent.net",
			".avito.ru", ".avito.st", ".gosuslugi.ru", ".mos.ru", ".nalog.ru", ".nalog.gov.ru",
			".sberbank.ru", ".sber.ru", ".tbank.ru", ".tinkoff.ru", ".tinkoffcdn.ru", ".cdn-tinkoff.ru", ".t-static.ru",
			".alfabank.ru", ".vtb.ru", ".gazprombank.ru", ".nspk.ru", ".rshb.ru",
			".2gis.ru", ".2gis.com", ".2gisapis.ru", ".rzd.ru", ".tutu.ru", ".aviasales.ru",
			".hh.ru", ".superjob.ru", ".rutube.ru", ".rutubelist.ru", ".ivi.ru", ".ivi.tv", ".okko.tv",
		}},
}

func IsRegionalService(id string) bool { return id == VKServiceID || id == RussianServiceID }
func RegionalServices() []Service      { return cloneServices(regionalServices) }
func VisibleServicesForSettings(settings RoutingSettings) []Service {
	if NormalizeMode(string(settings.Mode)) == ModeAll {
		return RegionalServices()
	}
	catalog, err := verifiedSettingsCatalog(settings.CatalogEnvelope)
	if err != nil {
		return RegionalServices()
	}
	return append(catalog, RegionalServices()...)
}
func VisibleServices(mode Mode) []Service {
	if NormalizeMode(string(mode)) == ModeAll {
		return RegionalServices()
	}
	return append(ServiceCatalog(), RegionalServices()...)
}

func (settings RoutingSettings) ServiceUsesVPN(id string) bool {
	if IsRegionalService(id) {
		mode := NormalizeMode(string(settings.Mode))
		if values := settings.ServiceRoutes[mode]; values != nil {
			if enabled, ok := values[id]; ok {
				return enabled
			}
		}
		return mode == ModeAll
	}
	if NormalizeMode(string(settings.Mode)) == ModeAll {
		return true
	}
	for _, selected := range settings.SelectedApps {
		if selected == id {
			return true
		}
	}
	return false
}

func (settings RoutingSettings) WithServiceVPN(id string, enabled bool) RoutingSettings {
	routes := make(map[Mode]map[string]bool, len(settings.ServiceRoutes)+1)
	for mode, values := range settings.ServiceRoutes {
		copyValues := map[string]bool{}
		for key, value := range values {
			copyValues[key] = value
		}
		routes[mode] = copyValues
	}
	mode := NormalizeMode(string(settings.Mode))
	if routes[mode] == nil {
		routes[mode] = map[string]bool{}
	}
	routes[mode][id] = enabled
	settings.ServiceRoutes = routes
	return settings
}

func HasDirectServiceExceptions(settings RoutingSettings) bool {
	if NormalizeMode(string(settings.Mode)) != ModeAll {
		return false
	}
	return !settings.ServiceUsesVPN(VKServiceID) || !settings.ServiceUsesVPN(RussianServiceID)
}

func regionalRuleObjects(settings RoutingSettings) []map[string]any {
	var rules []map[string]any
	for _, service := range RegionalServices() {
		outbound := "direct"
		if settings.ServiceUsesVPN(service.ID) {
			outbound = "awg-out"
		}
		domains := append([]string(nil), service.Domains...)
		for _, suffix := range service.DomainSuffixes {
			domains = append(domains, strings.TrimPrefix(suffix, "."))
		}
		// Process matching captures direct-IP media in the dedicated clients.
		// Browser WebRTC peer IPs cannot safely be attributed to VK globally.
		if len(service.ProcessNames) > 0 {
			rules = append(rules, map[string]any{"process_name": service.ProcessNames, "action": "route", "outbound": outbound})
		}
		rules = append(rules, map[string]any{"domain": domains, "domain_suffix": service.DomainSuffixes, "action": "route", "outbound": outbound})
	}
	return rules
}

func RegionalCatalogText() string {
	text := "Состав групп от " + RegionalCatalogRevision + ". Включено — VPN; выключено — напрямую.\nVK имеет приоритет над общей группой. Ваши правила имеют приоритет над обеими группами.\n\n"
	for _, service := range RegionalServices() {
		text += service.Name + ":\n" + strings.Join(append(service.Domains, service.DomainSuffixes...), ", ") + "\n\n"
	}
	return text + "Внешние сайты, облачные хранилища третьих сторон и неизвестные CDN не включаются автоматически. DNS прямых доменных исключений идёт к резолверам физической сети. DNS остальных запросов — через VPN. Сторонний DoH/ECH браузера может скрыть домен от маршрутизатора. Для звонков VK по прямому IP в браузере точная атрибуция недоступна: используйте правило на отдельный EXE приложения звонков. Общие UDP-порты и весь браузер эта группа не перенаправляет."
}
