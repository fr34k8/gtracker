package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jinzhu/now"
	_ "github.com/mattn/go-sqlite3"
	"github.com/syohex/go-texttable"

	"github.com/alexander-akhmetov/gtracker/common"
)

type appStats struct {
	Name        string
	RunningTime int
	Percentage  float64
}

type appStatsArray []appStats

func (a appStatsArray) Len() int {
	return len(a)
}

func (a appStatsArray) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

func (a appStatsArray) Less(i, j int) bool {
	return a[i].RunningTime < a[j].RunningTime
}

func lastMonthStats(args cmdArgs) {
	now.FirstDayMonday = true
	monthBeginningTimestamp := strconv.FormatInt(now.BeginningOfMonth().Unix(), 10)
	condition := fmt.Sprintf("startTime >= %s", monthBeginningTimestamp)
	getStatsForCondition(condition, args)
}

func lastWeekStats(args cmdArgs) {
	now.FirstDayMonday = true
	weekBeginningTimestamp := strconv.FormatInt(now.BeginningOfWeek().Unix(), 10)
	condition := fmt.Sprintf("startTime >= %s", weekBeginningTimestamp)
	getStatsForCondition(condition, args)
}

func todayStats(args cmdArgs) {
	todayBeginningTimestamp := strconv.FormatInt(now.BeginningOfDay().Unix(), 10)
	condition := fmt.Sprintf("startTime >= %s", todayBeginningTimestamp)
	getStatsForCondition(condition, args)
}

func yesterdayStats(args cmdArgs) {
	todayBeginningTimestamp := strconv.FormatInt(now.BeginningOfDay().Unix(), 10)
	yesterdayBeginningTimestamp := strconv.FormatInt(now.BeginningOfDay().Unix()-24*60*60, 10)
	condition := fmt.Sprintf("startTime >= %s AND endTime <= %s", yesterdayBeginningTimestamp, todayBeginningTimestamp)
	getStatsForCondition(condition, args)
}

func showForRange(args cmdArgs) {
	parsedStartDate, startDateError := now.Parse(args.StartDate)
	parsedEndDate, endDateError := now.Parse(args.EndDate)
	if startDateError != nil && endDateError != nil {
		log.Fatal("Error parsing time range")
	}
	condition := getCondition(parsedStartDate, startDateError, parsedEndDate, endDateError)
	if !args.GroupByDay {
		getStatsForCondition(condition, args)
	} else {
		for {
			condition = getCondition(parsedStartDate, startDateError, parsedEndDate, endDateError)
			getStatsForCondition(condition, args)
			if parsedStartDate.After(parsedEndDate.Add(time.Hour * 24 * -1)) {
				getStatsForCondition(condition, args)
				break
			}
			parsedEndDate = parsedEndDate.Add(time.Hour * 24 * -1)
		}
	}
}

func getCondition(parsedStartDate time.Time, startDateError error, parsedEndDate time.Time, endDateError error) string {
	condition := ""
	if startDateError == nil {
		condition = fmt.Sprintf("startTime >= %s", strconv.FormatInt(parsedStartDate.Unix(), 10))
	}
	if startDateError == nil && endDateError == nil {
		condition = condition + " AND"
	}
	if endDateError == nil {
		condition = fmt.Sprintf("%s endTime <= %s", condition, strconv.FormatInt(parsedEndDate.Unix(), 10))
	}
	return condition
}

func getStatsForCondition(whereCondition string, args cmdArgs) {
	db, err := sql.Open("sqlite3", path.Join(common.GetWorkDir(), databaseName))
	defer db.Close()
	groupKey := "name"
	if args.GroupByWindow {
		groupKey = "windowName"
	}
	filterQueryPart := ""
	if args.FilterByName != "" {
		filterQueryPart = fmt.Sprintf("%s %s", filterQueryPart, "AND name LIKE '%"+args.FilterByName+"%'")
	}
	if args.FilterByWindow != "" {
		filterQueryPart = fmt.Sprintf("%s %s", filterQueryPart, "AND windowName LIKE '%"+args.FilterByWindow+"%'")
	}
	var queryStr = fmt.Sprintf("SELECT name, windowName, SUM(runningTime), (SELECT SUM(runningTime) from apps WHERE %s %s) total FROM apps WHERE %s %s", whereCondition, filterQueryPart, whereCondition, filterQueryPart)
	queryStr = fmt.Sprintf("%s GROUP BY %s", queryStr, groupKey)
	rows, err := db.Query(queryStr)
	common.CheckError(err)
	statsArray := make([]appStats, 0)
	for rows.Next() {
		var name string
		var windowName string
		var runningTime float64
		var totalTime float64
		rows.Scan(&name, &windowName, &runningTime, &totalTime)
		nameStr := name
		if args.GroupByWindow {
			nameStr = windowName
		}
		if args.Formatter != "json" {
			if len(nameStr) > args.MaxNameLength {
				nameStr = nameStr[:args.MaxNameLength]
			}
		}
		statsArray = append(statsArray, appStats{Name: nameStr, RunningTime: int(runningTime), Percentage: float64(runningTime) / totalTime * 100})
	}
	formatters := map[string]func(statsArray []appStats){
		"pretty": statsPrettyTablePrinter,
		"simple": statsSimplePrinter,
		"json":   statsJsonPrinter,
	}
	sort.Sort(sort.Reverse(appStatsArray(statsArray)))
	if len(statsArray) < args.MaxResults {
		args.MaxResults = len(statsArray)
	}
	formatters[args.Formatter](statsArray[:args.MaxResults])
}

func statsPrettyTablePrinter(statsArray []appStats) {
	tbl := &texttable.TextTable{}
	tbl.SetHeader("Name", "Duration", "Percentage")
	for _, app := range statsArray {
		if app.Name != "" && app.RunningTime != 0 {
			tbl.AddRow(app.Name, getDurationString(app.RunningTime), getPercentageString(app.Percentage))
		}
	}
	fmt.Println(tbl.Draw())
}

func statsSimplePrinter(statsArray []appStats) {
	result := "Name\tDuration\tPercentage\n"
	for _, app := range statsArray {
		if app.Name != "" && app.RunningTime != 0 {
			result += fmt.Sprintf("%s\t%s\t%s\n", app.Name, getDurationString(app.RunningTime), getPercentageString(app.Percentage))
		}
	}
	fmt.Println(strings.TrimSuffix(result, "\n"))
}

func statsJsonPrinter(statsArray []appStats) {
	resultBytes, _ := json.Marshal(statsArray)
	resultStr := string(resultBytes)
	fmt.Println(resultStr)
}

func getPercentageString(percentage float64) string {
	return fmt.Sprintf("%.2f", percentage)
}

func getDurationString(runningTime int) string {
	hours, minutes, seconds := getTimeInfoFromDuration(runningTime)
	return fmt.Sprintf("%vh %vm %vs", hours, minutes, seconds)
}

func getTimeInfoFromDuration(duration int) (int, int, int) {
	hours := duration / 3600
	minutes := (duration / 60) - (hours * 60)
	seconds := duration - (minutes * 60) - (hours * 60 * 60)
	return hours, minutes, seconds
}
