package main

import (
	"flag"
	"fmt"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/antongulenko/golib"
	"github.com/bitflow-stream/go-bitflow/bitflow"
	"github.com/bitflow-stream/go-bitflow/script/reg"
	"github.com/gocolly/colly/v2"
	log "github.com/sirupsen/logrus"
)

type DataCollector struct {
	bitflow.AbstractSampleSource
	loopTask             golib.LoopTask
	samples              chan bitflow.SampleAndHeader
	sourceName           string
	loopWaitTime         time.Duration
	bufferedSamples      int
	fatalForwardingError bool
	ScrappedMetric       string
	TimeVal              time.Time
}

type Samples struct {
	imageName string
	Header    []string
	Metric    []bitflow.Value
	tags      map[string]string
	TimeVal   time.Time
}

// Assert that the DataCollector type implements the necessary interface
var _ bitflow.SampleSource = new(DataCollector)

func NewDataCollector() *DataCollector {
	col := &DataCollector{
		// TODO Initialize default parameter values here

		loopWaitTime:         10000 * time.Millisecond,
		bufferedSamples:      100,
		fatalForwardingError: false,
	}
	var myslice []string
	if err := col.Initialize(myslice); err != nil {
		log.Errorln("Initialization failed:", err)
		return col
	}
	col.loopTask.Loop = col.loopIteration
	return col
}

var SampleGeneratorParameters = reg.RegisteredParameters{}.
	Required("loopWaitTime", reg.Duration()).
	Required("bufferedSamples", reg.Int()).
	Required("sourceName", reg.String()).
	Required("fatalForwardingError", reg.Bool())

func (p *DataCollector) SetValues(params map[string]interface{}) {
	p.loopWaitTime = params["loopWaitTime"].(time.Duration)
	p.bufferedSamples = params["bufferedSamples"].(int)
	p.fatalForwardingError = params["fatalForwardingError"].(bool)
	p.sourceName = params["sourceName"].(string)
}

func (col *DataCollector) RegisterFlags() {
	flag.DurationVar(&col.loopWaitTime, "loop", col.loopWaitTime, "Time to wait in the main sample generation loop")
	flag.IntVar(&col.bufferedSamples, "sample-buffer", col.bufferedSamples, "Number of samples to buffer between the sample generation and sample forwarding routine")
	flag.BoolVar(&col.fatalForwardingError, "fatal-forwarding-error", col.fatalForwardingError, "Shutdown when there is an error forwarding a generated sample")
}

func (col *DataCollector) Initialize(args []string) error {
	col.samples = make(chan bitflow.SampleAndHeader, col.bufferedSamples)
	col.loopTask.Description = fmt.Sprintf("LoopTask of %v", col)
	return nil
}

func (col *DataCollector) String() string {
	// return a descriptive string, including any relevant parameters
	return fmt.Sprintf("Example data collector skeleton (loop frequency: %v, buffered samples: %v, fatal forwarding errors: %v)",
		col.loopWaitTime, col.bufferedSamples, col.fatalForwardingError)
}

func (col *DataCollector) Start(wg *sync.WaitGroup) golib.StopChan {
	stopper := col.loopTask.Start(wg)
	wg.Add(1)
	// Start a parallel routine to output samples, to decouple sample generation from sample sending
	go col.parallelForwardSamples(wg)
	return stopper
}

func (col *DataCollector) loopIteration(stopper golib.StopChan) error {
	if err := collectData(col); err != nil {
		return err
	}

	if err := col.generateSamples(); err != nil {
		return err
	}

	stopper.WaitTimeout(col.loopWaitTime)
	return nil
}

func collectData(metricData *DataCollector) error {
	c := colly.NewCollector()

	c.OnResponse(func(r *colly.Response) {
		//	fmt.Println("Visited", r.Request.URL)
		metricData.ScrappedMetric = string(r.Body)
		metricData.TimeVal = time.Now()

	})
	// Before making a request print "Visiting ..."
	c.OnRequest(func(r *colly.Request) {
	})

	// Start scraping on http://130.149.249.182:9283/metrics
	c.Visit("http://130.149.249.182:9283/metrics")

	return nil
}

func checkSample(samples []Samples, imageName string) int {
	for i := 0; i < len(samples); i++ {
		if samples[i].imageName == imageName {
			return i
		}
	}
	return -1
}

func (col *DataCollector) generateSamples() error {
	// This method is called in a loop, so samples can be generated in regular intervals. Alternatively, this method can be long-running and generate samples over a long period of time.
	// If the header does not change, the *bitflow.Header instance should be reused (unlike in this example).
	// Returning a non-nil error will cause the program to terminate, so non-fatal errors should be logged instead. The number of header fields and sample values must match, while the number of tags is arbitrary.
	pattern := "(((?P<ceph_rbd_read_latency_count>ceph_rbd_read_latency_count)|(?P<ceph_rbd_write_bytes>ceph_rbd_write_bytes)|(?P<ceph_rbd_write_ops>ceph_rbd_write_ops)|(?P<ceph_rbd_read_ops>ceph_rbd_read_ops)|(?P<ceph_rbd_write_latency_count>ceph_rbd_write_latency_count)|(?P<ceph_rbd_write_latency_sum>ceph_rbd_write_latency_sum)|(?P<ceph_rbd_read_bytes>ceph_rbd_read_bytes)|(?P<ceph_rbd_read_latency_sum>ceph_rbd_read_latency_sum)){(pool=(?P<pool>\"[A-Za-z]*\")?)?,?(namespace=(?P<namespace>\"[A-Za-z]*\")?)?,?(image=\"(?P<image>[A-Za-z0-9-]*)?\")?} (?P<value>[0-9.]*))"
	regExp := regexp.MustCompile(pattern)
	matched := regExp.FindAllStringSubmatch(col.ScrappedMetric, -1)
	var sampleArr []Samples
	for i := 0; i < len(matched); i++ {
		index := checkSample(sampleArr, matched[i][16])
		metricVal, err := strconv.ParseFloat(matched[i][17], 64)
		if err != nil {
			panic(err)
		}
		if index != -1 {
			sampleArr[index].Metric = append(sampleArr[index].Metric, bitflow.Value(metricVal))
			sampleArr[index].Header = append(sampleArr[index].Header, matched[i][2])
		} else if index == -1 {
			var sampleObj Samples
			sampleObj.Header = append(sampleObj.Header, matched[i][2])
			sampleObj.imageName = matched[i][16]
			sampleObj.Metric = append(sampleObj.Metric, bitflow.Value(metricVal))

			sampleObj.TimeVal = col.TimeVal
			sampleObj.tags = map[string]string{
				"pool":       matched[i][12],
				"namespace":  matched[i][14],
				"image":      matched[i][16],
				"sourceName": col.sourceName,
			}

			sampleArr = append(sampleArr, sampleObj)
		}
	}
	for i := 0; i < len(sampleArr); i++ {

		exampleHeader := &bitflow.Header{
			Fields: sampleArr[i].Header,
		}

		exampleSample := &bitflow.Sample{
			Time:   sampleArr[i].TimeVal,
			Values: sampleArr[i].Metric,
		}
		for k, v := range sampleArr[i].tags {
			exampleSample.SetTag(k, v)
		}
		col.emitSample(exampleSample, exampleHeader)
	}
	return nil
}

func (col *DataCollector) emitSample(sample *bitflow.Sample, header *bitflow.Header) {
	col.samples <- bitflow.SampleAndHeader{Sample: sample, Header: header}
}

func (col *DataCollector) Close() {
	// This method is called when the DataSource (including the parallel LoopTask) is asked to stop producing samples.
	// We stop our LoopTask, which will also command the fork parallelForwardSamples() to terminate.
	// Some cleanup tasks could be performed in this method, but it is recommended to do this in the cleanup() method.
	col.loopTask.Stop()
}

func (col *DataCollector) cleanup() {

	// TODO if necessary, perform additional cleanup routines and gracefully close resources
	// This method is called, after our last sample has been forwarded and all parallel routines have closed down.
	// Make sure to not panic here.

}

func (col *DataCollector) parallelForwardSamples(wg *sync.WaitGroup) {
	defer wg.Done()
	defer col.cleanup()
	defer col.CloseSinkParallel(wg)
	waitChan := col.loopTask.WaitChan()
	for {
		select {
		case <-waitChan:
			return
		case sample := <-col.samples:
			err := col.GetSink().Sample(sample.Sample, sample.Header)
			if err != nil {
				err = fmt.Errorf("Failed to forward data sample (number of values: %v): %v", len(sample.Sample.Values), err)
				if col.fatalForwardingError {
					col.loopTask.StopErr(err)
				} else {
					log.Errorln(err)
				}
			}
		}
	}
}
