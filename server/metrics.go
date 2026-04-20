package main

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Counter for total requests
	requestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pdf_requests_total",
			Help: "Total number of PDF generation requests",
		},
		[]string{"path", "status"},
	)

	// Histogram for request duration
	requestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "pdf_request_duration_seconds",
			Help:    "Time taken to process PDF generation requests",
			Buckets: []float64{.1, .5, 1, 2.5, 5, 10, 20, 30},
		},
		[]string{"path"},
	)

	// Gauge for current active requests
	activeRequests = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "pdf_active_requests",
			Help: "Number of currently active PDF generation requests",
		},
	)

	// Counter for errors
	errorTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "pdf_errors_total",
			Help: "Total number of PDF generation errors",
		},
		[]string{"type", "error"},
	)

	// Histogram for PDF file sizes
	pdfSize = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "pdf_size_bytes",
			Help:    "Size of generated PDFs in bytes",
			Buckets: prometheus.ExponentialBuckets(1024, 2, 10), // Starting from 1KB
		},
	)

	imageRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "image_requests_total",
			Help: "Total number of image generation requests",
		},
		[]string{"path", "status"},
	)

	imageRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "image_request_duration_seconds",
			Help:    "Time taken to process image generation requests",
			Buckets: []float64{.1, .5, 1, 2.5, 5, 10, 20, 30},
		},
		[]string{"path"},
	)

	imageActiveRequests = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "image_active_requests",
			Help: "Number of currently active image generation requests",
		},
	)

	imageErrorTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "image_errors_total",
			Help: "Total number of image generation errors",
		},
		[]string{"type", "error"},
	)

	imageSize = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "image_size_bytes",
			Help:    "Size of generated images in bytes",
			Buckets: prometheus.ExponentialBuckets(1024, 2, 12),
		},
	)
)
