package smartdns

import (
	"context"
	"fmt"
	"github.com/coredns/coredns/plugin"
	clog "github.com/coredns/coredns/plugin/pkg/log"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
	"strings"
	"time"
)

var log = clog.NewWithPlugin("smartdns")

// ServeDNS implements the plugin.Handler interface.
func (redis *Redis) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
	log.Debugf("客户端ip:%s", w.RemoteAddr().String())
	var clientIP string

	// Check for edns-client-subnet option
	for _, extra := range r.Extra {
		if opt, ok := extra.(*dns.OPT); ok {
			for _, s := range opt.Option {
				if ecs, ok := s.(*dns.EDNS0_SUBNET); ok {
					clientIP = ecs.Address.String()
					log.Debugf("客户端ip (from edns-client-subnet): %s", clientIP)
					break
				}
			}
		}
	}

	// If no ECS option is found, use remote address
	if clientIP == "" {
		clientIP = strings.Split(w.RemoteAddr().String(), ":")[0]
		log.Debugf("客户端ip (from remote address): %s", clientIP)
		ipa, err := Find(clientIP)
		if err != nil {
			ctx = context.WithValue(ctx, "public", "local")
			log.Debugf("客户端ip归属查询失败: %s  ", err)
		} else {
			ips := strings.Split(ipa, "|")
			if ips[0] == "0" {
				ctx = context.WithValue(ctx, "public", "local")
			} else {
				ctx = context.WithValue(ctx, "public", "public")
			}
			log.Debugf("客户端clientIP归属: %s  ", ipa)
		}
		ctx = handleClientIP(ctx, ipa, w.RemoteAddr().String())

	} else {
		ip := strings.Split(w.RemoteAddr().String(), ":")[0]
		log.Debugf("客户端ip : %s ednsip:%s ", ip, clientIP)
		ipa, err := Find(ip)
		if err != nil {
			log.Debugf("客户端ip归属查询失败: %s  ", err)
			log.Debugf("ServerHandle客户端ip归属: %s  ", ipa)
			ctx = context.WithValue(ctx, "public", "local")
			ctx = context.WithValue(ctx, "clientIp", ip)
		} else {
			eIps, err := Find(clientIP)
			if err != nil {
				log.Debugf("ednsip归属查询失败: %s  ", err)
				log.Debugf("edns: %s  ", eIps)
			}
			ctx = setClientIPContext(ctx, "eIps", eIps)
			ctx = setSmartContext(ctx, eIps)
		}
	}

	log.Debugf("ServeDNS called with query for: %s", r.Question[0].Name)
	state := request.Request{W: w, Req: r}

	qname := state.Name()
	qtype := state.Type()
	qtypeLower := strings.ToLower(qtype)

	if time.Since(redis.LastZoneUpdate) > zoneUpdateTime {
		redis.LoadZones()
	}

	zone := plugin.Zones(redis.Zones).Matches(qname)
	if zone == "" {
		return plugin.NextOrFailure(qname, redis.Next, ctx, w, r)
	}
	//增加智能解析
	z := redis.load(zone)
	if z == nil {
		return redis.errorResponse(state, zone, dns.RcodeServerFailure, nil)
	}
	log.Debugf("redis get zone: %s %s", z.Name, qname)
	location := redis.findLocation(qname, z, qtypeLower)
	if len(location) == 0 { // 无结果
		//if redis.Fallthrough {
		//	log.Debugf("fallthrough 已启用且未找到记录: %s，转到下一个插件", qname)
		//	return plugin.NextOrFailure(qname, redis.Next, ctx, w, r)
		//}
		//return redis.errorResponse(state, zone, dns.RcodeNameError, nil)
		log.Debugf("No record found for: %s, forwarding to next DNS server", qname)
		//return redis.Forward.ServeDNS(ctx, w, r) // Forward the query
		return redis.errorResponse(state, zone, dns.RcodeNameError, nil)
	}

	log.Debugf("找到记录: %s，处理查询", qname)

	answers := make([]dns.RR, 0, 10)
	extras := make([]dns.RR, 0, 10)
	ctx = context.WithValue(ctx, "qtype", qtypeLower)
	record := redis.get(ctx, location, z)

	switch qtype {
	case "A":
		answers, extras = redis.A(qname, z, record)
	case "AAAA":
		answers, extras = redis.AAAA(qname, z, record)
	case "CNAME":
		answers, extras = redis.CNAME(qname, z, record)
	case "TXT":
		answers, extras = redis.TXT(qname, z, record)
	case "NS":
		answers, extras = redis.NS(qname, z, record)
	case "MX":
		answers, extras = redis.MX(qname, z, record)
	case "SRV":
		answers, extras = redis.SRV(qname, z, record)
	case "SOA":
		answers, extras = redis.SOA(qname, z, record)
	case "CAA":
		answers, extras = redis.CAA(qname, z, record)
	default:
		return redis.errorResponse(state, zone, dns.RcodeNotImplemented, nil)
	}

	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative, m.RecursionAvailable, m.Compress = true, false, true

	m.Answer = append(m.Answer, answers...)
	m.Extra = append(m.Extra, extras...)

	state.SizeAndDo(m)
	m = state.Scrub(m)
	_ = w.WriteMsg(m)
	return dns.RcodeSuccess, nil
}

// Name implements the Handler interface.
func (redis *Redis) Name() string { return "smartdns" }

func (redis *Redis) errorResponse(state request.Request, zone string, rcode int, err error) (int, error) {
	m := new(dns.Msg)
	m.SetRcode(state.Req, rcode)
	m.Authoritative, m.RecursionAvailable, m.Compress = true, false, true

	state.SizeAndDo(m)
	_ = state.W.WriteMsg(m)
	// Return success as the rcode to signal we have written to the client.
	return dns.RcodeSuccess, err
}

// setClientIPContext sets the client IP context values
func setClientIPContext(ctx context.Context, key, value string) context.Context {
	return context.WithValue(ctx, key, value)
}

// setSmartContext sets the smart context values based on the IP address
func setSmartContext(ctx context.Context, ipa string) context.Context {
	ednsIp := strings.Split(ipa, "|")
	if len(ednsIp) > 4 {
		smartHash := fmt.Sprintf("%-%s", ednsIp[2], ednsIp[3])
		smartIsp := ednsIp[4]
		smartProvince := ednsIp[2]
		if ednsIp[0] == "0" {
			ctx = setClientIPContext(ctx, "public", "local")
		} else {
			ctx = setClientIPContext(ctx, "public", "public")
		}
		ctx = setClientIPContext(ctx, "smartHash", smartHash)
		ctx = setClientIPContext(ctx, "smartIsp", smartIsp)
		ctx = setClientIPContext(ctx, "smartProvince", smartProvince)
		ctx = setClientIPContext(ctx, "localNetwork", smartProvince)
	} else {
		log.Debugf("Invalid IP address format: %s", ipa)
	}
	return ctx
}

func handleClientIP(ctx context.Context, clientIP string, remoteAddr string) context.Context {
	log.Debugf("客户端ip: %s", clientIP)
	ipa, err := Find(clientIP)
	if err != nil {
		log.Debugf("客户端ip归属查询失败: %s", err)
	} else {
		log.Debugf("ClientIP归属: %s", ipa)
	}
	ctx = setClientIPContext(ctx, "clientIp", clientIP)
	return setSmartContext(ctx, ipa)
}
