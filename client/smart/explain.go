package smart

import (
	"fmt"
	"net"
	"path/filepath"
	"strings"
)

type RouteQuery struct{ Application, Domain, IP string }
type RouteExplanation struct {
	Target       RouteTarget
	Rule, Detail string
	MatchedRules []string
}

func (e RouteExplanation) String() string {
	target := "через VPN"
	if e.Target == TargetDirect {
		target = "напрямую"
	}
	text := fmt.Sprintf("Ожидаемый маршрут: %s\nПричина: %s\n%s", target, e.Rule, e.Detail)
	if len(e.MatchedRules) > 1 {
		text += "\nТакже совпали, но имеют меньший приоритет: " + strings.Join(e.MatchedRules[1:], ", ")
	}
	return text + "\n\nЭто расчёт по настройкам; фактический трафик не проверялся. Для доменных правил движку необходимо определить домен соединения."
}

func domainMatches(domain, rule string) bool {
	rule = strings.TrimPrefix(strings.TrimSuffix(strings.ToLower(rule), "."), ".")
	return domain == rule || strings.HasSuffix(domain, "."+rule)
}

// ExplainRoute uses the same first-match order as BuildConfig: explicit user
// rules, LAN bypass, selected service rules, update endpoints, final policy.
func ExplainRoute(settings RoutingSettings, query RouteQuery) (RouteExplanation, error) {
	settings, err := settings.Normalized()
	if err != nil {
		return RouteExplanation{}, err
	}
	query.Application = strings.Trim(strings.TrimSpace(query.Application), "\"")
	if query.Domain != "" {
		query.Domain, err = normalizeDomain(query.Domain)
		if err != nil {
			return RouteExplanation{}, err
		}
	}
	ip := net.ParseIP(strings.TrimSpace(query.IP))
	if query.IP != "" && ip == nil {
		return RouteExplanation{}, fmt.Errorf("некорректный IP-адрес")
	}
	if query.Application == "" && query.Domain == "" && ip == nil {
		return RouteExplanation{}, fmt.Errorf("укажи приложение, домен или IP")
	}
	result := RouteExplanation{Target: TargetVPN, Rule: "Весь интернет через VPN"}
	if settings.Mode == ModeSelected {
		result.Target = TargetDirect
		result.Rule = "Не входит в выбранное для VPN"
	}
	match := func(target RouteTarget, name, detail string) {
		if len(result.MatchedRules) == 0 {
			result.Target = target
			result.Rule = name
			result.Detail = detail
		}
		result.MatchedRules = append(result.MatchedRules, name)
	}
	for _, rule := range settings.CustomRules {
		if !rule.Enabled {
			continue
		}
		matched := false
		switch rule.Kind {
		case RuleApplication:
			if strings.ContainsAny(rule.Value, `\/:`) {
				matched = strings.EqualFold(filepath.Clean(query.Application), filepath.Clean(rule.Value))
			} else {
				matched = strings.EqualFold(filepath.Base(query.Application), rule.Value)
			}
		case RuleDomain:
			matched = query.Domain != "" && domainMatches(query.Domain, rule.Value)
		case RuleCIDR:
			_, network, e := net.ParseCIDR(rule.Value)
			matched = e == nil && ip != nil && network.Contains(ip)
		}
		if matched {
			match(rule.Target, rule.Name, "Пользовательское правило: "+rule.Value)
		}
	}

	for _, service := range RegionalServices() {
		matched := false
		for _, name := range service.ProcessNames {
			matched = matched || strings.EqualFold(filepath.Base(query.Application), name)
		}
		for _, domain := range service.Domains {
			matched = matched || query.Domain == domain
		}
		for _, suffix := range service.DomainSuffixes {
			matched = matched || (query.Domain != "" && domainMatches(query.Domain, suffix))
		}
		if matched {
			target := TargetDirect
			if settings.ServiceUsesVPN(service.ID) {
				target = TargetVPN
			}
			match(target, service.Name, "Каталог "+RegionalCatalogRevision+"; VK выше общей российской группы")
		}
	}
	if ip != nil {
		for _, cidr := range localCIDRs {
			_, network, _ := net.ParseCIDR(cidr)
			if network.Contains(ip) {
				match(TargetDirect, "Локальная сеть", cidr)
				break
			}
		}
	}
	if settings.Mode == ModeSelected {
		for _, service := range settings.SelectedServices() {
			matched := false
			for _, name := range service.ProcessNames {
				if strings.EqualFold(filepath.Base(query.Application), name) {
					matched = true
				}
			}
			for _, domain := range service.Domains {
				if query.Domain == domain {
					matched = true
				}
			}
			for _, suffix := range service.DomainSuffixes {
				if query.Domain != "" && domainMatches(query.Domain, suffix) {
					matched = true
				}
			}
			if matched {
				match(TargetVPN, service.Name, "Подписанный или встроенный каталог сервисов")
			}
		}
		if query.Domain == "github.com" || query.Domain == "raw.githubusercontent.com" {
			match(TargetVPN, "Обновления пресетов", "Служебное правило")
		}
	}
	return result, nil
}
