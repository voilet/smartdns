package smartdns

import (
	"fmt"
	"github.com/lionsoul2014/ip2region/binding/golang/xdb"
)

var (
	ipBuff []byte
)

// Region 示例查询结果
type Region struct {
	Country  string `json:"country"`
	Region   string `json:"region"`
	Province string `json:"province"`
	City     string `json:"city"`
	ISP      string `json:"isp"`
}

func (redis *Redis) InitIpDb() {
	if redis.IpdbPath != "" {
		var err error
		ipBuff, err = xdb.LoadContentFromFile(redis.IpdbPath)
		if err != nil {
			fmt.Printf("加载数据库数据失败 `%s`: %s\n", redis.IpdbPath, err)
			return
		}
	}
}

// func Find(ip string) (reg Region, e error) {
func Find(ip string) (s string, e error) {
	//var ipInfo Region
	searcher, err := xdb.NewWithBuffer(ipBuff)
	if err != nil {
		fmt.Printf("创建searcher失败: %s\n", err.Error())
		//return ipInfo, err
		return "", err
	}

	defer searcher.Close()

	info, err := searcher.SearchByStr(ip)
	if err != nil {
		fmt.Printf("查询ip失败(%s): %s\n", ip, err)
		//return ipInfo, e
		return "", e
	}

	//ips := strings.Split(info, "|")
	//ipInfo.Country = ips[0]
	//ipInfo.Region = ips[1]
	//ipInfo.Province = ips[2]
	//ipInfo.City = ips[3]
	//return ipInfo, nil
	return info, nil
}
