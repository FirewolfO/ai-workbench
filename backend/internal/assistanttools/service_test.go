package assistanttools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLocalDailyTools(t *testing.T) {
	service := New()
	service.now = func() time.Time { return time.Date(2026, 8, 27, 4, 5, 6, 0, time.UTC) }

	calculated, err := service.Execute(context.Background(), "calculate", json.RawMessage(`{"expression":"pow(1.05, 2) * 100"}`))
	if err != nil || !strings.Contains(calculated, `"result":110.25`) {
		t.Fatalf("calculate = %q, %v", calculated, err)
	}
	converted, err := service.Execute(context.Background(), "convert_unit", json.RawMessage(`{"value":100,"from":"F","to":"C"}`))
	if err != nil || !strings.Contains(converted, `"result":37.777`) {
		t.Fatalf("convert = %q, %v", converted, err)
	}
	current, err := service.Execute(context.Background(), "get_current_time", json.RawMessage(`{"timezone":"Asia/Shanghai"}`))
	if err != nil || !strings.Contains(current, `"date":"2026-08-27"`) || !strings.Contains(current, `"time":"12:05:06"`) {
		t.Fatalf("time = %q, %v", current, err)
	}
}

func TestRemoteDailyTools(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/geo":
			_, _ = writer.Write([]byte(`{"results":[{"name":"武汉","country":"中国","admin1":"湖北","latitude":30.59,"longitude":114.30,"timezone":"Asia/Shanghai"}]}`))
		case "/weather":
			_, _ = writer.Write([]byte(`{"timezone":"Asia/Shanghai","current":{"time":"2026-08-27T12:00","temperature_2m":32.4,"apparent_temperature":36.1,"weather_code":2,"wind_speed_10m":8.2},"daily":{"time":["2026-08-27"],"weather_code":[2],"temperature_2m_max":[34],"temperature_2m_min":[26],"precipitation_probability_max":[40]}}`))
		case "/exchange/latest":
			_, _ = writer.Write([]byte(`{"amount":100,"base":"CNY","date":"2026-08-26","rates":{"USD":13.92}}`))
		case "/holidays/2026/CN":
			_, _ = writer.Write([]byte(`[{"date":"2026-10-01","localName":"国庆节","name":"National Day","global":true}]`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	service := New()
	service.geocodingURL = server.URL + "/geo"
	service.weatherURL = server.URL + "/weather"
	service.exchangeURL = server.URL + "/exchange"
	service.holidayURL = server.URL + "/holidays"

	weather, err := service.Execute(context.Background(), "get_weather", json.RawMessage(`{"location":"武汉","days":1}`))
	if err != nil || !strings.Contains(weather, `"temperature_c":32.4`) || !strings.Contains(weather, `"weather":"多云"`) {
		t.Fatalf("weather = %q, %v", weather, err)
	}
	exchange, err := service.Execute(context.Background(), "get_exchange_rate", json.RawMessage(`{"base":"CNY","quote":"USD","amount":100}`))
	if err != nil || !strings.Contains(exchange, `"result":13.92`) {
		t.Fatalf("exchange = %q, %v", exchange, err)
	}
	holidays, err := service.Execute(context.Background(), "get_public_holidays", json.RawMessage(`{"country_code":"CN","year":2026}`))
	if err != nil || !strings.Contains(holidays, "国庆节") {
		t.Fatalf("holidays = %q, %v", holidays, err)
	}
}

func TestToolValidation(t *testing.T) {
	service := New()
	for _, test := range []struct {
		name string
		args string
	}{
		{"calculate", `{"expression":"1/0"}`},
		{"convert_unit", `{"value":1,"from":"kg","to":"km"}`},
		{"get_current_time", `{"timezone":"not/a-zone"}`},
		{"get_weather", `{"location":"","days":9}`},
		{"unknown", `{}`},
	} {
		if _, err := service.Execute(context.Background(), test.name, json.RawMessage(test.args)); err == nil {
			t.Fatalf("%s should reject %s", test.name, test.args)
		}
	}
}
