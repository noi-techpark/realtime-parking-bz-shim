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

var stations = []string{"\"103\"", "\"104\"", "\"105\"", "\"106\""}

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
	Scode  string `json:"scode"`
	Sname  string `json:"sname"`
	Mvalue T      `json:"mvalue"`
}

var threshold_str string = os.Getenv("THRESHOLD")
var threshold int

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

	var err error
	threshold, err = strconv.Atoi(threshold_str)
	if err != nil {
		slog.Error("Error while parsing threshold from env", err)
	}

	r.Use(gin.Recovery())

	r.GET("/", shim)
	r.GET("/health", health)
	r.Run()
}
func health(c *gin.Context) {
	c.Status(http.StatusOK)
}

func shim(c *gin.Context) {
	var res []interface{}

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

		if ts < now-p.Mperiod*2*1000 {
			res = append(res, ParkingResponse[string]{Scode: p.Scode, Sname: p.Sname, Mvalue: "--"})
		} else if free < int32(threshold) {
			res = append(res, ParkingResponse[int32]{Scode: p.Scode, Sname: p.Sname, Mvalue: 0})
		} else {
			res = append(res, ParkingResponse[int32]{Scode: p.Scode, Sname: p.Sname, Mvalue: free})
		}
	}

	c.JSON(http.StatusOK, res)
}

func getOdhParking() ([]OdhParking, error) {
	req := ninja.DefaultNinjaRequest()
	req.Limit = -1
	req.StationTypes = []string{"ParkingStation"}

	req.Where = "and(sactive.eq.true,scode.in.(" + strings.Join(stations, ",") + "))"
	req.DataTypes = []string{"occupied"}

	var res ninja.NinjaResponse[[]OdhParking]
	err := ninja.Latest(req, &res)
	return res.Data, err
}
