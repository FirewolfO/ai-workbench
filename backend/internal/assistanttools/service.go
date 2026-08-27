package assistanttools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"ai-workbench/internal/llm"
)

const (
	defaultGeocodingURL = "https://geocoding-api.open-meteo.com/v1/search"
	defaultWeatherURL   = "https://api.open-meteo.com/v1/forecast"
	defaultExchangeURL  = "https://api.frankfurter.dev/v1"
	defaultHolidayURL   = "https://date.nager.at/api/v3/PublicHolidays"
)

type Service struct {
	client       *http.Client
	geocodingURL string
	weatherURL   string
	exchangeURL  string
	holidayURL   string
	now          func() time.Time
}

func New() *Service {
	return &Service{
		client: &http.Client{Timeout: 15 * time.Second}, geocodingURL: defaultGeocodingURL,
		weatherURL: defaultWeatherURL, exchangeURL: defaultExchangeURL, holidayURL: defaultHolidayURL,
		now: time.Now,
	}
}

func (s *Service) Definitions() []llm.ToolDefinition {
	object := func(properties map[string]any, required ...string) map[string]any {
		return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
	}
	return []llm.ToolDefinition{
		{
			Name: "get_weather", Description: "查询指定城市或地区当前天气和未来 1 至 7 天天气预报。用户询问天气、温度、下雨概率、穿衣或出行天气时使用。",
			Parameters: object(map[string]any{
				"location": map[string]any{"type": "string", "description": "城市或地区名称，例如北京、武汉、Shanghai"},
				"days":     map[string]any{"type": "integer", "minimum": 1, "maximum": 7, "description": "预报天数，今天填 1"},
			}, "location", "days"),
		},
		{
			Name: "get_current_time", Description: "查询指定 IANA 时区的当前日期、时间和星期。涉及今天日期、当前时间或跨时区换算时使用。",
			Parameters: object(map[string]any{
				"timezone": map[string]any{"type": "string", "description": "IANA 时区，例如 Asia/Shanghai、Europe/London；中国时间使用 Asia/Shanghai"},
			}, "timezone"),
		},
		{
			Name: "calculate", Description: "精确计算数学表达式，支持加减乘除、括号和 abs、sqrt、pow、min、max、round、floor、ceil 函数。不要心算可由此工具完成的数值。",
			Parameters: object(map[string]any{
				"expression": map[string]any{"type": "string", "description": "数学表达式，例如 (128.5*3-20)/2 或 pow(1.05,12)"},
			}, "expression"),
		},
		{
			Name: "convert_unit", Description: "换算常用长度、面积、质量、体积、速度和温度单位。",
			Parameters: object(map[string]any{
				"value": map[string]any{"type": "number", "description": "要换算的数值"},
				"from":  map[string]any{"type": "string", "description": "原单位，如 km、kg、C、mph、acre、L"},
				"to":    map[string]any{"type": "string", "description": "目标单位，如 mi、lb、F、km/h、m2、mL"},
			}, "value", "from", "to"),
		},
		{
			Name: "get_exchange_rate", Description: "查询实时参考汇率并换算金额。涉及外币、汇率或旅行预算时使用。",
			Parameters: object(map[string]any{
				"base":   map[string]any{"type": "string", "description": "三位源货币代码，如 CNY、USD、EUR"},
				"quote":  map[string]any{"type": "string", "description": "三位目标货币代码，如 CNY、USD、JPY"},
				"amount": map[string]any{"type": "number", "description": "源货币金额"},
			}, "base", "quote", "amount"),
		},
		{
			Name: "get_public_holidays", Description: "查询某个国家或地区指定年份的公共节假日。安排休假、旅行或工作日时使用。",
			Parameters: object(map[string]any{
				"country_code": map[string]any{"type": "string", "description": "ISO 3166-1 两位国家代码，如 CN、US、JP"},
				"year":         map[string]any{"type": "integer", "minimum": 2000, "maximum": 2100, "description": "年份"},
			}, "country_code", "year"),
		},
	}
}

func (s *Service) Execute(ctx context.Context, name string, arguments json.RawMessage) (string, error) {
	switch name {
	case "get_weather":
		var input struct {
			Location string `json:"location"`
			Days     int    `json:"days"`
		}
		if err := decodeArguments(arguments, &input); err != nil {
			return "", err
		}
		return s.weather(ctx, input.Location, input.Days)
	case "get_current_time":
		var input struct {
			Timezone string `json:"timezone"`
		}
		if err := decodeArguments(arguments, &input); err != nil {
			return "", err
		}
		return s.currentTime(input.Timezone)
	case "calculate":
		var input struct {
			Expression string `json:"expression"`
		}
		if err := decodeArguments(arguments, &input); err != nil {
			return "", err
		}
		return calculate(input.Expression)
	case "convert_unit":
		var input struct {
			Value float64 `json:"value"`
			From  string  `json:"from"`
			To    string  `json:"to"`
		}
		if err := decodeArguments(arguments, &input); err != nil {
			return "", err
		}
		return convertUnit(input.Value, input.From, input.To)
	case "get_exchange_rate":
		var input struct {
			Base   string  `json:"base"`
			Quote  string  `json:"quote"`
			Amount float64 `json:"amount"`
		}
		if err := decodeArguments(arguments, &input); err != nil {
			return "", err
		}
		return s.exchange(ctx, input.Base, input.Quote, input.Amount)
	case "get_public_holidays":
		var input struct {
			CountryCode string `json:"country_code"`
			Year        int    `json:"year"`
		}
		if err := decodeArguments(arguments, &input); err != nil {
			return "", err
		}
		return s.holidays(ctx, input.CountryCode, input.Year)
	default:
		return "", fmt.Errorf("未知后台工具 %q", name)
	}
}

func decodeArguments(arguments json.RawMessage, target any) error {
	if len(arguments) == 0 || !json.Valid(arguments) {
		return errors.New("工具参数不是有效 JSON")
	}
	if err := json.Unmarshal(arguments, target); err != nil {
		return fmt.Errorf("工具参数无效: %w", err)
	}
	return nil
}

func (s *Service) currentTime(timezone string) (string, error) {
	timezone = strings.TrimSpace(timezone)
	if timezone == "" || len(timezone) > 80 {
		return "", errors.New("时区不能为空")
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return "", fmt.Errorf("无效 IANA 时区 %q", timezone)
	}
	now := s.now().In(location)
	zone, offset := now.Zone()
	return marshalResult(map[string]any{
		"timezone": timezone, "date": now.Format("2006-01-02"), "time": now.Format("15:04:05"),
		"weekday": now.Weekday().String(), "zone": zone, "utc_offset_seconds": offset, "iso8601": now.Format(time.RFC3339),
	})
}

func (s *Service) weather(ctx context.Context, location string, days int) (string, error) {
	location = strings.TrimSpace(location)
	if location == "" || len([]rune(location)) > 100 || days < 1 || days > 7 {
		return "", errors.New("地点或预报天数无效")
	}
	query := url.Values{"name": {location}, "count": {"1"}, "language": {"zh"}, "format": {"json"}}
	var geocoding struct {
		Results []struct {
			Name      string  `json:"name"`
			Country   string  `json:"country"`
			Admin1    string  `json:"admin1"`
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
			Timezone  string  `json:"timezone"`
		} `json:"results"`
	}
	geocodingTarget := s.geocodingURL + "?" + query.Encode()
	if err := s.getJSON(ctx, geocodingTarget, &geocoding); err != nil {
		return "", fmt.Errorf("地点查询失败: %w", err)
	}
	if len(geocoding.Results) == 0 {
		return "", fmt.Errorf("没有找到地点 %q", location)
	}
	place := geocoding.Results[0]
	forecastQuery := url.Values{
		"latitude": {strconv.FormatFloat(place.Latitude, 'f', 6, 64)}, "longitude": {strconv.FormatFloat(place.Longitude, 'f', 6, 64)},
		"current":  {"temperature_2m,apparent_temperature,weather_code,wind_speed_10m"},
		"daily":    {"weather_code,temperature_2m_max,temperature_2m_min,precipitation_probability_max"},
		"timezone": {"auto"}, "forecast_days": {strconv.Itoa(days)},
	}
	var forecast struct {
		Timezone string `json:"timezone"`
		Current  struct {
			Time                string  `json:"time"`
			Temperature         float64 `json:"temperature_2m"`
			ApparentTemperature float64 `json:"apparent_temperature"`
			WeatherCode         int     `json:"weather_code"`
			WindSpeed           float64 `json:"wind_speed_10m"`
		} `json:"current"`
		Daily struct {
			Time                     []string  `json:"time"`
			WeatherCode              []int     `json:"weather_code"`
			TemperatureMax           []float64 `json:"temperature_2m_max"`
			TemperatureMin           []float64 `json:"temperature_2m_min"`
			PrecipitationProbability []int     `json:"precipitation_probability_max"`
		} `json:"daily"`
	}
	forecastTarget := s.weatherURL + "?" + forecastQuery.Encode()
	if err := s.getJSON(ctx, forecastTarget, &forecast); err != nil {
		return "", fmt.Errorf("天气查询失败: %w", err)
	}
	daily := make([]map[string]any, 0, len(forecast.Daily.Time))
	for index, date := range forecast.Daily.Time {
		if index >= len(forecast.Daily.WeatherCode) || index >= len(forecast.Daily.TemperatureMax) || index >= len(forecast.Daily.TemperatureMin) || index >= len(forecast.Daily.PrecipitationProbability) {
			break
		}
		daily = append(daily, map[string]any{
			"date": date, "weather": weatherDescription(forecast.Daily.WeatherCode[index]),
			"temperature_max_c": forecast.Daily.TemperatureMax[index], "temperature_min_c": forecast.Daily.TemperatureMin[index],
			"precipitation_probability_percent": forecast.Daily.PrecipitationProbability[index],
		})
	}
	return marshalResult(map[string]any{
		"location": map[string]any{"name": place.Name, "admin1": place.Admin1, "country": place.Country, "latitude": place.Latitude, "longitude": place.Longitude},
		"timezone": forecast.Timezone, "observed_at": forecast.Current.Time,
		"current": map[string]any{
			"weather": weatherDescription(forecast.Current.WeatherCode), "temperature_c": forecast.Current.Temperature,
			"apparent_temperature_c": forecast.Current.ApparentTemperature, "wind_speed_kmh": forecast.Current.WindSpeed,
		},
		"forecast": daily, "source": forecastTarget,
	})
}

func (s *Service) exchange(ctx context.Context, base, quote string, amount float64) (string, error) {
	base = strings.ToUpper(strings.TrimSpace(base))
	quote = strings.ToUpper(strings.TrimSpace(quote))
	validCurrency := regexp.MustCompile(`^[A-Z]{3}$`)
	if !validCurrency.MatchString(base) || !validCurrency.MatchString(quote) || math.IsNaN(amount) || math.IsInf(amount, 0) || math.Abs(amount) > 1e12 {
		return "", errors.New("货币代码或金额无效")
	}
	if base == quote {
		return marshalResult(map[string]any{"date": s.now().Format("2006-01-02"), "base": base, "quote": quote, "amount": amount, "result": amount, "rate": 1})
	}
	query := url.Values{"from": {base}, "to": {quote}, "amount": {strconv.FormatFloat(amount, 'f', -1, 64)}}
	target := strings.TrimRight(s.exchangeURL, "/") + "/latest?" + query.Encode()
	var response struct {
		Amount float64            `json:"amount"`
		Base   string             `json:"base"`
		Date   string             `json:"date"`
		Rates  map[string]float64 `json:"rates"`
	}
	if err := s.getJSON(ctx, target, &response); err != nil {
		return "", fmt.Errorf("汇率查询失败: %w", err)
	}
	result, ok := response.Rates[quote]
	if !ok {
		return "", fmt.Errorf("汇率服务不支持 %s/%s", base, quote)
	}
	rate := 0.0
	if amount != 0 {
		rate = result / amount
	}
	return marshalResult(map[string]any{"date": response.Date, "base": base, "quote": quote, "amount": amount, "result": result, "rate": rate, "source": target})
}

func (s *Service) holidays(ctx context.Context, countryCode string, year int) (string, error) {
	countryCode = strings.ToUpper(strings.TrimSpace(countryCode))
	if !regexp.MustCompile(`^[A-Z]{2}$`).MatchString(countryCode) || year < 2000 || year > 2100 {
		return "", errors.New("国家代码或年份无效")
	}
	target := strings.TrimRight(s.holidayURL, "/") + "/" + strconv.Itoa(year) + "/" + countryCode
	var holidays []struct {
		Date       string `json:"date"`
		LocalName  string `json:"localName"`
		Name       string `json:"name"`
		Global     bool   `json:"global"`
		LaunchYear *int   `json:"launchYear"`
	}
	if err := s.getJSON(ctx, target, &holidays); err != nil {
		return "", fmt.Errorf("节假日查询失败: %w", err)
	}
	return marshalResult(map[string]any{"country_code": countryCode, "year": year, "holidays": holidays, "source": target})
}

func (s *Service) getJSON(ctx context.Context, target string, result any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "AI-Workbench/1.0")
	response, err := s.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		return fmt.Errorf("上游返回 %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(result); err != nil {
		return errors.New("上游返回了无效数据")
	}
	return nil
}

func calculate(expression string) (string, error) {
	expression = strings.TrimSpace(expression)
	if expression == "" || len(expression) > 500 {
		return "", errors.New("计算表达式无效")
	}
	parsed, err := parser.ParseExpr(expression)
	if err != nil {
		return "", fmt.Errorf("无法解析计算表达式: %w", err)
	}
	value, err := evaluate(parsed)
	if err != nil {
		return "", err
	}
	if math.IsInf(value, 0) || math.IsNaN(value) {
		return "", errors.New("计算结果不是有限数值")
	}
	return marshalResult(map[string]any{"expression": expression, "result": value, "formatted": strconv.FormatFloat(value, 'f', -1, 64)})
}

func evaluate(expression ast.Expr) (float64, error) {
	switch value := expression.(type) {
	case *ast.BasicLit:
		if value.Kind != token.INT && value.Kind != token.FLOAT {
			return 0, errors.New("表达式只能包含数值")
		}
		return strconv.ParseFloat(value.Value, 64)
	case *ast.ParenExpr:
		return evaluate(value.X)
	case *ast.UnaryExpr:
		operand, err := evaluate(value.X)
		if err != nil {
			return 0, err
		}
		switch value.Op {
		case token.ADD:
			return operand, nil
		case token.SUB:
			return -operand, nil
		default:
			return 0, errors.New("不支持该一元运算")
		}
	case *ast.BinaryExpr:
		left, err := evaluate(value.X)
		if err != nil {
			return 0, err
		}
		right, err := evaluate(value.Y)
		if err != nil {
			return 0, err
		}
		switch value.Op {
		case token.ADD:
			return left + right, nil
		case token.SUB:
			return left - right, nil
		case token.MUL:
			return left * right, nil
		case token.QUO:
			if right == 0 {
				return 0, errors.New("不能除以零")
			}
			return left / right, nil
		default:
			return 0, errors.New("仅支持加减乘除")
		}
	case *ast.CallExpr:
		identifier, ok := value.Fun.(*ast.Ident)
		if !ok {
			return 0, errors.New("函数无效")
		}
		arguments := make([]float64, 0, len(value.Args))
		for _, argument := range value.Args {
			result, err := evaluate(argument)
			if err != nil {
				return 0, err
			}
			arguments = append(arguments, result)
		}
		return evaluateFunction(identifier.Name, arguments)
	default:
		return 0, errors.New("表达式包含不支持的内容")
	}
}

func evaluateFunction(name string, values []float64) (float64, error) {
	one := func(function func(float64) float64) (float64, error) {
		if len(values) != 1 {
			return 0, fmt.Errorf("%s 需要 1 个参数", name)
		}
		return function(values[0]), nil
	}
	switch strings.ToLower(name) {
	case "abs":
		return one(math.Abs)
	case "sqrt":
		if len(values) != 1 || values[0] < 0 {
			return 0, errors.New("sqrt 需要 1 个非负参数")
		}
		return math.Sqrt(values[0]), nil
	case "round":
		return one(math.Round)
	case "floor":
		return one(math.Floor)
	case "ceil":
		return one(math.Ceil)
	case "pow":
		if len(values) != 2 {
			return 0, errors.New("pow 需要 2 个参数")
		}
		return math.Pow(values[0], values[1]), nil
	case "min", "max":
		if len(values) < 2 {
			return 0, fmt.Errorf("%s 至少需要 2 个参数", name)
		}
		result := values[0]
		for _, value := range values[1:] {
			if (name == "min" && value < result) || (name == "max" && value > result) {
				result = value
			}
		}
		return result, nil
	default:
		return 0, fmt.Errorf("不支持函数 %q", name)
	}
}

type unit struct {
	dimension string
	factor    float64
	offset    float64
}

var units = map[string]unit{
	"mm": {"length", 0.001, 0}, "cm": {"length", 0.01, 0}, "m": {"length", 1, 0}, "km": {"length", 1000, 0},
	"in": {"length", 0.0254, 0}, "ft": {"length", 0.3048, 0}, "yd": {"length", 0.9144, 0}, "mi": {"length", 1609.344, 0},
	"mg": {"mass", 0.000001, 0}, "g": {"mass", 0.001, 0}, "kg": {"mass", 1, 0}, "oz": {"mass", 0.028349523125, 0}, "lb": {"mass", 0.45359237, 0}, "jin": {"mass", 0.5, 0},
	"m2": {"area", 1, 0}, "cm2": {"area", 0.0001, 0}, "km2": {"area", 1e6, 0}, "ha": {"area", 10000, 0}, "acre": {"area", 4046.8564224, 0}, "ft2": {"area", 0.09290304, 0}, "mu": {"area", 2000.0 / 3.0, 0},
	"ml": {"volume", 0.001, 0}, "l": {"volume", 1, 0}, "m3": {"volume", 1000, 0}, "floz": {"volume", 0.0295735295625, 0}, "cup": {"volume", 0.2365882365, 0}, "gal": {"volume", 3.785411784, 0},
	"m/s": {"speed", 1, 0}, "km/h": {"speed", 0.2777777777778, 0}, "mph": {"speed", 0.44704, 0}, "knot": {"speed", 0.5144444444444, 0},
	"c": {"temperature", 1, 0}, "f": {"temperature", 5.0 / 9.0, -32.0 * 5.0 / 9.0}, "k": {"temperature", 1, -273.15},
}

var unitAliases = map[string]string{
	"毫米": "mm", "厘米": "cm", "米": "m", "千米": "km", "公里": "km", "英寸": "in", "英尺": "ft", "英里": "mi",
	"毫克": "mg", "克": "g", "千克": "kg", "公斤": "kg", "斤": "jin", "磅": "lb",
	"毫升": "ml", "升": "l", "摄氏度": "c", "华氏度": "f", "开尔文": "k", "℃": "c", "℉": "f",
	"平方米": "m2", "平方公里": "km2", "公顷": "ha", "亩": "mu", "公里/小时": "km/h", "米/秒": "m/s",
}

func convertUnit(value float64, from, to string) (string, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return "", errors.New("换算数值无效")
	}
	from = normalizeUnit(from)
	to = normalizeUnit(to)
	source, sourceOK := units[from]
	target, targetOK := units[to]
	if !sourceOK || !targetOK || source.dimension != target.dimension {
		return "", fmt.Errorf("不支持从 %q 换算到 %q", from, to)
	}
	base := value*source.factor + source.offset
	result := (base - target.offset) / target.factor
	return marshalResult(map[string]any{"value": value, "from": from, "to": to, "result": result, "formatted": strconv.FormatFloat(result, 'f', -1, 64)})
}

func normalizeUnit(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "²", "2")
	value = strings.ReplaceAll(value, "³", "3")
	value = strings.ReplaceAll(value, " ", "")
	if alias := unitAliases[value]; alias != "" {
		return alias
	}
	return value
}

func weatherDescription(code int) string {
	switch {
	case code == 0:
		return "晴"
	case code == 1:
		return "大部晴朗"
	case code == 2:
		return "多云"
	case code == 3:
		return "阴"
	case code == 45 || code == 48:
		return "雾"
	case code >= 51 && code <= 57:
		return "毛毛雨"
	case code >= 61 && code <= 67:
		return "雨"
	case code >= 71 && code <= 77:
		return "雪"
	case code >= 80 && code <= 82:
		return "阵雨"
	case code >= 85 && code <= 86:
		return "阵雪"
	case code >= 95:
		return "雷暴"
	default:
		return fmt.Sprintf("天气代码 %d", code)
	}
}

func marshalResult(value any) (string, error) {
	data, err := json.Marshal(value)
	return string(data), err
}
