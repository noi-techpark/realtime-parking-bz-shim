// SPDX-FileCopyrightText: NOI Techpark <digital@noi.bz.it>
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
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

type OdhParking struct {
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
	r.Run()
}

func health(c *gin.Context) {
	c.Status(http.StatusOK)
}

func shim(c *gin.Context) {
	res := ninja.NinjaResponse[[]any]{Offset: 0, Limit: 200}

	parking, err := getOdhParking()
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

// shimV2 supports:
//   - ?threshold=N  override the zero-display threshold (defaults to DEFAULT_THRESHOLD env)
//   - ?where=...    ODH where-filter passed through as-is (defaults to sactive.eq.true)
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

	whereFilter := c.Query("where")
	if whereFilter == "" {
		whereFilter = "sactive.eq.true"
	}

	res := ninja.NinjaResponse[[]any]{Offset: 0, Limit: 200}

	parking, err := getOdhParkingV2(whereFilter)
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

func getOdhParking() ([]OdhParking, error) {
	req := ninja.DefaultNinjaRequest()
	req.Limit = -1
	req.StationTypes = []string{"ParkingStation"}

	req.Where = "and(sactive.eq.true,scode.in.(" + stationString + "))"
	req.DataTypes = []string{"occupied"}

	var res ninja.NinjaResponse[[]OdhParking]
	err := ninja.Latest(req, &res)
	return res.Data, err
}

func getOdhParkingV2(whereFilter string) ([]OdhParking, error) {
	req := ninja.DefaultNinjaRequest()
	req.Limit = -1
	req.StationTypes = []string{"ParkingStation"}
	req.Where = whereFilter
	req.DataTypes = []string{"occupied"}

	var res ninja.NinjaResponse[[]OdhParking]
	err := ninja.Latest(req, &res)
	return res.Data, err
}
