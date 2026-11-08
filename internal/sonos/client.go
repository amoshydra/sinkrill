package sonos

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const soapTimeout = 5 * time.Second

type service struct {
	id         string
	controlURL string
}

var (
	avTransportService = service{
		id:         "urn:schemas-upnp-org:service:AVTransport:1",
		controlURL: "/MediaRenderer/AVTransport/Control",
	}
	renderingService = service{
		id:         "urn:schemas-upnp-org:service:RenderingControl:1",
		controlURL: "/MediaRenderer/RenderingControl/Control",
	}
)

type Client struct {
	ip   string
	http *http.Client
}

func NewClient(ip string) *Client {
	return &Client{ip: ip, http: &http.Client{Timeout: soapTimeout}}
}

func (c *Client) call(ctx context.Context, svc service, action string, args [][2]string) ([]byte, error) {
	var body bytes.Buffer
	body.WriteString(`<?xml version="1.0" encoding="utf-8"?>`)
	body.WriteString(`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" `)
	body.WriteString(`s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body>`)
	fmt.Fprintf(&body, `<u:%s xmlns:u="%s">`, action, svc.id)
	for _, arg := range args {
		fmt.Fprintf(&body, "<%s>", arg[0])
		if err := xml.EscapeText(&body, []byte(arg[1])); err != nil {
			return nil, err
		}
		fmt.Fprintf(&body, "</%s>", arg[0])
	}
	fmt.Fprintf(&body, `</u:%s>`, action)
	body.WriteString(`</s:Body></s:Envelope>`)

	url := "http://" + c.ip + ":1400" + svc.controlURL
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SOAPACTION", fmt.Sprintf(`"%s#%s"`, svc.id, action))

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sonos %s: HTTP %d: %s", action, resp.StatusCode, snippet(data))
	}
	return data, nil
}

// SetAVTransportURI points the speaker at a new URI (it does not start playback).
func (c *Client) SetAVTransportURI(ctx context.Context, uri, metadata string) error {
	_, err := c.call(ctx, avTransportService, "SetAVTransportURI", [][2]string{
		{"InstanceID", "0"},
		{"CurrentURI", uri},
		{"CurrentURIMetaData", metadata},
	})
	return err
}

func (c *Client) Play(ctx context.Context) error {
	_, err := c.call(ctx, avTransportService, "Play", [][2]string{
		{"InstanceID", "0"},
		{"Speed", "1"},
	})
	return err
}

func (c *Client) Stop(ctx context.Context) error {
	_, err := c.call(ctx, avTransportService, "Stop", [][2]string{
		{"InstanceID", "0"},
	})
	return err
}

type transportInfoResponse struct {
	State string `xml:"Body>GetTransportInfoResponse>CurrentTransportState"`
}

// TransportState returns PLAYING, PAUSED_PLAYBACK, STOPPED, TRANSITIONING...
func (c *Client) TransportState(ctx context.Context) (string, error) {
	data, err := c.call(ctx, avTransportService, "GetTransportInfo", [][2]string{
		{"InstanceID", "0"},
	})
	if err != nil {
		return "", err
	}
	var parsed transportInfoResponse
	if err := xml.Unmarshal(data, &parsed); err != nil {
		return "", err
	}
	return parsed.State, nil
}

type positionInfoResponse struct {
	RelTime       string `xml:"Body>GetPositionInfoResponse>RelTime"`
	TrackDuration string `xml:"Body>GetPositionInfoResponse>TrackDuration"`
	TrackURI      string `xml:"Body>GetPositionInfoResponse>TrackURI"`
}

type Position struct {
	Seconds  int
	Duration int
	URI      string
}

func (c *Client) Position(ctx context.Context) (Position, error) {
	data, err := c.call(ctx, avTransportService, "GetPositionInfo", [][2]string{
		{"InstanceID", "0"},
	})
	if err != nil {
		return Position{}, err
	}
	var parsed positionInfoResponse
	if err := xml.Unmarshal(data, &parsed); err != nil {
		return Position{}, err
	}
	return Position{
		Seconds:  ParseRelTime(parsed.RelTime),
		Duration: ParseRelTime(parsed.TrackDuration),
		URI:      parsed.TrackURI,
	}, nil
}

func (c *Client) SetVolume(ctx context.Context, volume int) error {
	_, err := c.call(ctx, renderingService, "SetVolume", [][2]string{
		{"InstanceID", "0"},
		{"Channel", "Master"},
		{"DesiredVolume", strconv.Itoa(volume)},
	})
	return err
}

type volumeResponse struct {
	Volume string `xml:"Body>GetVolumeResponse>CurrentVolume"`
}

func (c *Client) Volume(ctx context.Context) (int, error) {
	data, err := c.call(ctx, renderingService, "GetVolume", [][2]string{
		{"InstanceID", "0"},
		{"Channel", "Master"},
	})
	if err != nil {
		return 0, err
	}
	var parsed volumeResponse
	if err := xml.Unmarshal(data, &parsed); err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(parsed.Volume))
}

func snippet(data []byte) string {
	text := strings.Join(strings.Fields(string(data)), " ")
	if len(text) > 300 {
		text = text[:300] + "..."
	}
	return text
}
