package jdecode

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/sirupsen/logrus"
	"github.com/toukii/goutils"
	"github.com/toukii/jsnm"
)

type iv map[string]interface{}

var (
	at      = "@"[0]
	comma   = rune(","[0])
	dblquot = rune(`"`[0])
	dollar  = rune(`$`[0])
	ranger  = "$range"

	log *logrus.Entry
)

func init() {
	SetLog("debug")
}

func SetLog(logLevel string) {
	lvl, err := logrus.ParseLevel(logLevel)
	if err != nil {
		logrus.Error(err)
		lvl = logrus.DebugLevel
	}
	logger := logrus.New()
	logger.SetLevel(lvl)
	log = logrus.NewEntry(logger)
}

func jsonen(i interface{}) ([]byte, error) {
	return json.Marshal(i)
}

type RangePath struct {
	prefixPaths, suffixPaths []string
	ranged                   bool // 是否循环
}

func TrimPath(paths []string) RangePath {
	ret := make([]string, len(paths))
	for i, it := range paths {
		if len(it) <= 0 {
			ret[i] = ""
		} else {
			ret[i] = it
		}
	}

	for i, it := range paths {
		if it == ranger {
			return RangePath{
				prefixPaths: ret[:i],
				suffixPaths: ret[i+1:],
				ranged:      true,
			}
		}
	}

	return RangePath{prefixPaths: ret}
}

func decodeRange(js *jsnm.Jsnm, raw, pathStr string) ([]string, string) {
	arr := js.Arr()
	retsize := len(arr)
	if retsize <= 0 {
		return nil, ""
	}
	ret := make([]string, retsize)
	for i, it := range arr {
		var v string
		bs, err := jsonen(it.RawData().Raw())
		if err == nil {
			v = string(bs)
		} else {
			v = fmt.Sprint(it.RawData().Raw())
		}

		ret[i] = strings.Replace(raw, fmt.Sprintf(`"@%s"`, pathStr), v, 1)
	}
	return ret, pathStr
}

func Decode(raw string, prebs []byte) ([]string, string) {
	if raw == "" {
		return []string{""}, ""
	}
	ret := raw
	js := jsnm.BytesFmt(prebs)
	allpaths := subDecode(raw, true)
	if len(allpaths) == 1 && allpaths[0] == "" {
		return []string{strings.Replace(raw, `"@"`, goutils.ToString(prebs), 1)}, allpaths[0]
	}
	for _, it := range allpaths {
		rawpaths := strings.Split(it, ",")
		rangePaths := TrimPath(rawpaths)
		size := len(rawpaths)
		if size <= 0 {
			continue
		}
		rawArrGet := js.ArrGet(rangePaths.prefixPaths...)
		val := rawArrGet.RawData().Raw()
		if val == nil {
			continue
		}
		if rangePaths.ranged {
			arr := rawArrGet.Arr()
			retsize := len(arr)
			ret := make([]string, retsize)
			for i, item := range arr {
				var v string
				bs, err := jsonen(item.ArrGet(rangePaths.suffixPaths...).RawData().Raw())
				if err == nil {
					v = string(bs)
				} else {
					v = fmt.Sprint(item.ArrGet(rangePaths.suffixPaths...).RawData().Raw())
				}

				ret[i] = strings.Replace(raw, fmt.Sprintf(`"@%s"`, it), v, 1)
			}
			return ret, it
		}
		vv, typ := value(val)
		if vv != "" {
			if typ != "string" && strings.Contains(ret, fmt.Sprintf(`"@%s"`, it)) {
				ret = strings.Replace(ret, fmt.Sprintf(`"@%s"`, it), fmt.Sprintf(`%s`, vv), -1)
			}
			ret = strings.Replace(ret, fmt.Sprintf(`"@%s`, it), fmt.Sprintf(`"%s`, vv), -1)
		}
	}
	return []string{ret}, ""
}

func subDecode(raw interface{}, first bool) []string {
	if first {
		var vals interface{}
		err := json.Unmarshal([]byte(fmt.Sprint(raw)), &vals)
		if err != nil {
			log.Errorf("%+v, err:%+v", raw, err)
			return nil
		}
		return decodeMap(vals)
	}
	switch typ := raw.(type) {
	case string:
		if retlet, ok := getLetterStr([]byte(fmt.Sprint(raw))); ok {
			return []string{retlet}
		}
	case []interface{}:
		return decodeSlice(raw)
	case map[string]interface{}:
		return decodeMap(raw)
	default:
		log.Debugf("%+v decode unsupported!", typ)
	}
	return nil
}

func decodeMap(raw interface{}) []string {
	vs, ok := raw.(map[string]interface{})
	if !ok {
		return nil
	}
	ret := make([]string, 0, 1)
	for _, subit := range vs {
		if subret := subDecode(subit, false); len(subret) > 0 {
			ret = append(ret, subret...)
		}
	}
	return ret
}

func decodeSlice(raw interface{}) []string {
	vs, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	ret := make([]string, 0, 1)
	for _, subit := range vs {
		if subret := subDecode(subit, false); len(subret) > 0 {
			ret = append(ret, subret...)
		}
	}
	return ret
}

func value(v interface{}) (string, string) {
	switch typ := v.(type) {
	case int:
		return fmt.Sprint(v.(int)), "int"
	case int32:
		return fmt.Sprint(v.(int32)), "int32"
	case int64:
		return fmt.Sprint(v.(int64)), "int64"
	case float32:
		vv := v.(float32)
		return fmt.Sprint(int64(vv)), "float32"
	case float64:
		vv := v.(float64)
		return fmt.Sprint(vv * 1.0), "float64"
	case string:
		return fmt.Sprint(v), "string"
	default:
		log.Infof("%+v value unsupported!", typ)
	}
	return "", "unsupport"
}

// 返回符合jsnm ArrGet的路径，以@开头,以#结尾
func getLetterStr(bs []byte) (string, bool) {
	if len(bs) <= 0 {
		return "", false
	}
	if bs[0] != at {
		return "", false
	}
	rs := bytes.Runes(bs)
	size := len(rs)
	for i := 1; i < size; i++ {
		if unicode.IsLetter(rs[i]) || unicode.IsNumber(rs[i]) || rs[i] == comma || rs[i] == dblquot || rs[i] == dollar {
			continue
		}
		return goutils.ToString(bs[1:i]), true
	}
	return goutils.ToString(bs[1:]), true
}
