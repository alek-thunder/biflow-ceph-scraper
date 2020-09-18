package main

import (
	"errors"
	"fmt"
	"log"
	"reflect"
	"strings"

	"github.com/antongulenko/golib"
	"github.com/bitflow-stream/go-bitflow/bitflow"
)

type setMapValue func(*bitflow.Sample, *bitflow.Header)

type DataSamples struct {
	header *bitflow.Header
	sample *bitflow.Sample
}

func (d *DataSamples) setValues(Sample *bitflow.Sample, Header *bitflow.Header) {
	d.sample = Sample.Clone()
	d.header = Header.Clone(Header.Fields)
}

func newSample() *DataSamples {
	sample := &DataSamples{
		header: &bitflow.Header{
			Fields: []string{},
		},
		sample: &bitflow.Sample{},
	}
	return sample
}

type SyncSampleProcessor struct {
	bitflow.NoopProcessor
	PrintModulo      int
	cephSamples      map[string]*DataSamples
	openStackSamples map[string]*DataSamples
	task             golib.LoopTask
	samplesGenerated int
}

var _ bitflow.SampleProcessor = new(SyncSampleProcessor)

func (p *SyncSampleProcessor) String() string {
	return fmt.Sprintf("Mock processor (log every %v sample(s))", p.PrintModulo)
}

func (p *SyncSampleProcessor) Sample(sample *bitflow.Sample, header *bitflow.Header) error {
	if p.PrintModulo > 0 && p.samplesGenerated%p.PrintModulo == 0 {
		log.Println("Mock processor processing sample nr", p.samplesGenerated)
	}
	mergedSample, err := p.synchronizeSamples(sample, header)
	if err != nil {
		p.samplesGenerated++
		return p.NoopProcessor.Sample(mergedSample.sample, mergedSample.header)
	} else {
		p.samplesGenerated++
		return p.NoopProcessor.Sample(mergedSample.sample, mergedSample.header)
	}
}

// this is the function that decides what sample it received
// and then calls the merge function to perform the actual merge
func (p *SyncSampleProcessor) synchronizeSamples(sample *bitflow.Sample, header *bitflow.Header) (*DataSamples, error) {
	var err error
	var mergedSample = newSample()
	if image, ok_image := sample.TagMap()["image"]; ok_image {
		// when sample is from ceph
		if (p.cephSamples == nil) || (p.cephSamples[image] == nil) || (sample.Time != p.cephSamples[image].sample.Time) {
			p.cephSamples[image] = newSample()
			p.cephSamples[image].setValues(sample, header)
			mergedSample, err = p.samplesMerge(mergedSample, image)
			if err != nil {
				fmt.Println(err)
				return mergedSample, err
			}
		}
	} else if _, ok_vm := sample.TagMap()["vm"]; ok_vm {
		// when sample is from openstack
		vol := sample.TagMap()["volumes"]
		linked_Volumes := strings.Split(vol, "|")
		for _, volume := range linked_Volumes {
			p.openStackSamples[volume] = newSample()
			p.openStackSamples[volume].setValues(sample, header)
		}
		mergedSample, err = p.samplesMerge(mergedSample, vol)
		if err != nil {
			fmt.Println(err)
			return mergedSample, err
		}
	}
	return mergedSample, nil
}

// This function selects openstack sample as the primary sample
// and then calls the mergeAll function for ceph sample to be merged
func (p *SyncSampleProcessor) samplesMerge(mergedSample *DataSamples, image string) (*DataSamples, error) {
	vols := strings.Split(image, "|")
	image = vols[0]
	if p.openStackSamples[image] == nil || reflect.DeepEqual(p.openStackSamples[image], mergedSample) {
		return mergedSample, errors.New("can't merge an empty struct")
	} else {
		mergedSample.header = p.openStackSamples[image].header.Clone(p.openStackSamples[image].header.Fields)
		mergedSample.sample = p.openStackSamples[image].sample.Clone()
		var err error
		values := strings.Split(p.openStackSamples[image].sample.TagMap()["volumes"], "|")
		for _, image_param := range values {
			mergedSample, err = p.mergeAll(mergedSample, image_param)
			if err != nil {
				return mergedSample, err
			}
		}

	}
	return mergedSample, nil
}

// This function merges the matching ceph samples into the primary openstack sample
func (p *SyncSampleProcessor) mergeAll(mergedSample *DataSamples, image string) (*DataSamples, error) {
	if p.cephSamples[image] == nil || reflect.DeepEqual(p.cephSamples[image], newSample()) {
		return mergedSample, errors.New("no such volume found in ceph")
	} else if p.cephSamples[image].sample.Values == nil {
		return mergedSample, errors.New("no such volume found in ceph")
	} else {
		for i := 0; i < len(p.cephSamples[image].header.Fields); i++ {
			mergedSample.header.Fields = append(mergedSample.header.Fields, "ceph/"+image+"/"+p.cephSamples[image].header.Fields[i])
			mergedSample.sample.Values = append(mergedSample.sample.Values, p.cephSamples[image].sample.Values[i])
		}
		for k, v := range p.cephSamples[image].sample.TagMap() {
			k = "ceph/" + image + "/" + k
			mergedSample.sample.SetTag(k, v)
		}
	}
	return mergedSample, nil
}
