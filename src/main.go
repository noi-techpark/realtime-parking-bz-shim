// SPDX-FileCopyrightText: NOI Techpark <digital@noi.bz.it>
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	_ "embed"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	sloggin "github.com/samber/slog-gin"

	"opendatahub/realtime-parking-bz-shim/ninja"
)

//go:embed docs/swagger.yaml
var openapiSpec []byte

//go:embed docs/redoc.html
var redocPage []byte

// ParkingStation is a single station entry as returned by the shim.
type ParkingStation struct {
	Scode      string `json:"scode" example:"103"`
	Sname      string `json:"sname" example:"P03 - Piazza Walther"`
	Mvalue     int32  `json:"mvalue" example:"42"`
	Mvalidtime string `json:"mvalidtime" example:"2026-08-05T11:40:00.000+0000"`
}

// ShimResponse is the response envelope returned by the shim endpoints.
type ShimResponse struct {
	Offset int64            `json:"offset" example:"0"`
	Limit  int64            `json:"limit" example:"200"`
	Data   []ParkingStation `json:"data"`
}

type OpenDataHubParking struct {
	Scode   string `json:"scode"`
	Sname   string `json:"sname"`
	Sorigin string `json:"sorigin"`
	Scoord  struct {
		X    float32 `json:"x"`
		Y    float32 `json:"y"`
		Srid uint32  `json:"srid"`
	} `json:"scoordinate"`
	Smeta struct {
		StandardName        string `json:"standard_name"`
		Capacity            int32  `json:"capacity"`
		ParkingProhibitions bool   `json:"parkingprohibitions"`
		ParkingCharging     bool   `json:"parkingcharging"`
		ParkingSurveillance bool   `json:"parkingsurveillance"`
	} `json:"smetadata"`
	Mvalue     float64         `json:"mvalue"`
	Mperiod    int64           `json:"mperiod"`
	Mvalidtime ninja.NinjaTime `json:"mvalidtime"`
}

type ParkingResponse[T string | int32] struct {
	Scode      string `json:"scode"`
	Sname      string `json:"sname"`
	Mvalue     T      `json:"mvalue"`
	Mvalidtime string `json:"mvalidtime"`
}

var stationsCodesStr string = os.Getenv("STATION_CODES")
var defaultThresholdStr string = os.Getenv("DEFAULT_THRESHOLD")

var stationString string
var defaultThreshold int

// @title Realtime Parking Shim API
// @version 1.0
// @description.markdown api
// @BasePath /
// @externalDocs.description GitHub repository
// @externalDocs.url https://github.com/noi-techpark/realtime-parking-bz-shim
func main() {
	InitLogger()
	r := gin.New()

	if os.Getenv("GIN_LOG") == "PRETTY" {
		r.Use(gin.Logger())
	} else {
		// Enable slog logging for gin framework
		// https://github.com/samber/slog-gin
		r.Use(sloggin.New(slog.Default()))
	}

	// convert stationCodesStr from env to comma separated string with escape backticks
	stationCodes := strings.Split(stationsCodesStr, ",")
	for i, s := range stationCodes {
		stationString += "\"" + s + "\""
		if i < len(stationCodes)-1 {
			stationString += ","
		}
	}

	var err error
	defaultThreshold, err = strconv.Atoi(defaultThresholdStr)
	if err != nil {
		slog.Error("Error while parsing threshold from env", err)
	}

	r.Use(gin.Recovery())

	r.GET("/", shim)
	r.GET("/health", health)
	r.GET("/v2/", shimV2)
	r.GET("/openapi.yaml", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/yaml; charset=utf-8", openapiSpec)
	})
	r.GET("/docs", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", redocPage)
	})
	r.Run()
}

// health godoc
// @Summary Health check
// @Tags health
// @Success 200
// @Router /health [get]
func health(c *gin.Context) {
	c.Status(http.StatusOK)
}

// shim godoc
// @Summary Parking shim
// @Description Returns parking station availability for a pre-configured set of stations. This is the legacy version of this API and is considered deprecated
// @Tags parking
// @Produce json
// @Success 200 {object} ShimResponse
// @Failure 500
// @Deprecated
// @Router / [get]
func shim(c *gin.Context) {
	res := ninja.NinjaResponse[[]any]{Offset: 0, Limit: 200}

	parking, err := getOpenDataHubParking()
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
	}

	now := time.Now().UnixMilli()

	for _, p := range parking {
		ts := p.Mvalidtime.UnixMilli()

		// use occupied and calc free
		// because original data collector sends occupied,
		// so free gets calculated by elaboration, which creates delay of 5 minutes
		free := p.Smeta.Capacity - int32(p.Mvalue)

		// Super ugly hotfix to exclude laurin old timeseries with period 300
		if p.Scode == "105" && p.Mperiod == 300 {
			continue
		}

		if ts < now-p.Mperiod*2*1000 {
			// res.Data = append(res.Data, ParkingResponse[string]{Scode: p.Scode, Sname: p.Sname, Mvalidtime: p.Mvalidtime.Format(ninja.RequestTimeFormat), Mvalue: "--"})
			res.Data = append(res.Data, ParkingResponse[int32]{Scode: p.Scode, Sname: p.Sname, Mvalidtime: p.Mvalidtime.Format(ninja.RequestTimeFormat), Mvalue: -1})
		} else if free < int32(defaultThreshold) {
			res.Data = append(res.Data, ParkingResponse[int32]{Scode: p.Scode, Sname: p.Sname, Mvalidtime: p.Mvalidtime.Format(ninja.RequestTimeFormat), Mvalue: 0})
		} else if free > 999 {
			res.Data = append(res.Data, ParkingResponse[int32]{Scode: p.Scode, Sname: p.Sname, Mvalidtime: p.Mvalidtime.Format(ninja.RequestTimeFormat), Mvalue: 999})
		} else {
			res.Data = append(res.Data, ParkingResponse[int32]{Scode: p.Scode, Sname: p.Sname, Mvalidtime: p.Mvalidtime.Format(ninja.RequestTimeFormat), Mvalue: free})
		}

	}

	c.JSON(http.StatusOK, res)
}

// shimV2 godoc
// @Summary Parking shim (v2)
// @Description Returns parking availability filtered by a where-expression, with a configurable zero-display threshold.
// @Tags parking
// @Produce json
// @Param threshold query int false "Override the zero-display threshold (default: 10)"
// @Param where query string false "Where-filter, forwarded as-is to the upstream Timeseries API where parameter (see [the timeseries api swagger](https://swagger.opendatahub.com/?urls.primaryName=Timeseries+-+mobility.api.opendatahub.com#/Timeseries/get_v2__representation___stationTypes___dataTypes__latest)); sactive.eq.true is always appended"
// @Success 200 {object} ShimResponse
// @Failure 400
// @Failure 500
// @Router /v2/ [get]
func shimV2(c *gin.Context) {
	threshold := defaultThreshold
	if tStr := c.Query("threshold"); tStr != "" {
		t, err := strconv.Atoi(tStr)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid threshold parameter"})
			return
		}
		threshold = t
	}

	const defaultFilters = "sactive.eq.true"
	whereFilter := c.Query("where")
	if whereFilter != "" {
		whereFilter += ","
	}
	whereFilter += defaultFilters

	res := ninja.NinjaResponse[[]any]{Offset: 0, Limit: 200}

	parking, err := getOpenDataHubParkingV2(whereFilter)
	if err != nil {
		c.AbortWithError(http.StatusInternalServerError, err)
		return
	}

	now := time.Now().UnixMilli()

	for _, p := range parking {
		ts := p.Mvalidtime.UnixMilli()

		free := p.Smeta.Capacity - int32(p.Mvalue)

		// Super ugly hotfix to exclude laurin old timeseries with period 300
		if p.Scode == "105" && p.Mperiod == 300 {
			continue
		}

		if ts < now-p.Mperiod*2*1000 {
			res.Data = append(res.Data, ParkingResponse[int32]{Scode: p.Scode, Sname: p.Sname, Mvalidtime: p.Mvalidtime.Format(ninja.RequestTimeFormat), Mvalue: -1})
		} else if free < int32(threshold) {
			res.Data = append(res.Data, ParkingResponse[int32]{Scode: p.Scode, Sname: p.Sname, Mvalidtime: p.Mvalidtime.Format(ninja.RequestTimeFormat), Mvalue: 0})
		} else if free > 999 {
			res.Data = append(res.Data, ParkingResponse[int32]{Scode: p.Scode, Sname: p.Sname, Mvalidtime: p.Mvalidtime.Format(ninja.RequestTimeFormat), Mvalue: 999})
		} else {
			res.Data = append(res.Data, ParkingResponse[int32]{Scode: p.Scode, Sname: p.Sname, Mvalidtime: p.Mvalidtime.Format(ninja.RequestTimeFormat), Mvalue: free})
		}
	}

	c.JSON(http.StatusOK, res)
}

func getOpenDataHubParking() ([]OpenDataHubParking, error) {
	req := ninja.DefaultNinjaRequest()
	req.Limit = -1
	req.StationTypes = []string{"ParkingStation"}

	req.Where = "and(sactive.eq.true,scode.in.(" + stationString + "))"
	req.DataTypes = []string{"occupied"}

	var res ninja.NinjaResponse[[]OpenDataHubParking]
	err := ninja.Latest(req, &res)
	return res.Data, err
}

func getOpenDataHubParkingV2(whereFilter string) ([]OpenDataHubParking, error) {
	req := ninja.DefaultNinjaRequest()
	req.Limit = -1
	req.StationTypes = []string{"ParkingStation"}
	req.Where = whereFilter
	req.DataTypes = []string{"occupied"}

	var res ninja.NinjaResponse[[]OpenDataHubParking]
	err := ninja.Latest(req, &res)
	return res.Data, err
}
