package main

import (
	"fmt"

	"github.com/bitflow-stream/go-bitflow/bitflow"
	"github.com/bitflow-stream/go-bitflow/script/plugin"
	"github.com/bitflow-stream/go-bitflow/script/reg"
)

const (
	DataSourceType    = "ceph-metric-collector"
	DataProcessorName = "ceph-metric-collector"
)

// The Symbol to be loaded
var Plugin plugin.BitflowPlugin = new(pluginImpl)

type pluginImpl struct {
}

func (*pluginImpl) Name() string {
	return "ceph-metric-collector-plugin"
}

func (p *pluginImpl) Init(registry reg.ProcessorRegistry) error {

	plugin.LogPluginDataSource(p, DataSourceType)
	registry.Endpoints.CustomDataSources[bitflow.EndpointType(DataSourceType)] = func(endpointUrl string) (bitflow.SampleSource, error) {
		_, params, err := reg.ParseEndpointUrlParams(endpointUrl, SampleGeneratorParameters)
		if err != nil {
			return nil, err
		}
		generator := NewDataCollector()
		generator.SetValues(params)
		return generator, nil
	}
	plugin.LogPluginProcessor(p, DataProcessorName)
	registry.RegisterStep(DataProcessorName, func(pipeline *bitflow.SamplePipeline, params map[string]interface{}) error {
		pipeline.Add(&SyncSampleProcessor{
			PrintModulo:      params["print"].(int),
			cephSamples:      make(map[string]*DataSamples),
			openStackSamples: make(map[string]*DataSamples),
		})
		fmt.Println(pipeline)
		return nil
	}, "This step is to synchronize samples from openstack and ceph").
		Required("print", reg.Int())

	return nil
}
