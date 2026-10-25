package config

import "encoding/json"

// fields 保留直连引擎的字段访问，并集中定义它们在配置中的槽名。
func (k *APIKeys) fields() map[string]*string {
	return map[string]*string{
		"deepl": &k.DeepL, "baidu_appid": &k.BaiduAppID, "baidu_key": &k.BaiduKey,
		"niutrans": &k.NiuTrans, "ocrspace": &k.OCRSpace, "baidu_ocr_ak": &k.BaiduOCRAK,
		"baidu_ocr_sk": &k.BaiduOCRsk, "agnes": &k.Agnes, "qwen": &k.Qwen, "acctoken": &k.Acctoken,
	}
}

func (k APIKeys) Get(name string) string {
	if field := k.fields()[name]; field != nil {
		return *field
	}
	return k.Extra[name]
}

func (k *APIKeys) Set(name, value string) {
	if field := k.fields()[name]; field != nil {
		*field = value
		return
	}
	if k.Extra == nil {
		k.Extra = make(map[string]string)
	}
	k.Extra[name] = value
}

// Values 返回副本，供 JSON 编码和对外掩码处理使用。
func (k APIKeys) Values() map[string]string {
	values := make(map[string]string, len(k.Extra)+10)
	for name, value := range k.Extra {
		values[name] = value
	}
	for name, field := range k.fields() {
		values[name] = *field
	}
	return values
}

func (k APIKeys) MarshalJSON() ([]byte, error) { return json.Marshal(k.Values()) }

func (k *APIKeys) UnmarshalJSON(data []byte) error {
	var values map[string]*string
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}
	for name, value := range values {
		// 与原结构体解码一致：缺失和 null 保留默认值，空字符串明确覆盖。
		if value != nil {
			k.Set(name, *value)
		}
	}
	return nil
}
