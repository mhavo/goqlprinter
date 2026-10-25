//go:build usb

package brotherql

import (
	"fmt"
	"log"
	"log/slog"

	"github.com/google/gousb"
)

// USBBackend provides a BrotherQL backend for USB printers.
type USBBackend struct {
	ctx         *gousb.Context
	dev         *gousb.Device
	cfg         *gousb.Config
	intf        *gousb.Interface
	inEndpoint  *gousb.InEndpoint
	outEndpoint *gousb.OutEndpoint
}

// NewUSBBackend creates a new USBBackend instance.
func NewUSBBackend(dev *gousb.Device) (*USBBackend, error) {
	log.Printf("Info: USB device has %d configuration(s).", len(dev.Desc.Configs))
	for _, cfgDesc := range dev.Desc.Configs {
		log.Printf("  - Configuration #%d", cfgDesc.Number)
	}

	// Auto-detach the kernel usblp driver so libusb can claim the interface.
	if err := dev.SetAutoDetach(true); err != nil {
		slog.Warn("failed to set auto-detach on USB device", "error", err)
	}

	cfg, err := dev.Config(1)
	if err != nil {
		return nil, err
	}

	intf, err := cfg.Interface(0, 0)
	if err != nil {
		if cerr := cfg.Close(); cerr != nil {
			slog.Warn("failed to close USB config", "error", cerr)
		}
		return nil, err
	}

	var inEndpoint *gousb.InEndpoint
	var outEndpoint *gousb.OutEndpoint
	for _, desc := range intf.Setting.Endpoints {
		if desc.Direction == gousb.EndpointDirectionIn {
			inEndpoint, err = intf.InEndpoint(desc.Number)
		} else {
			outEndpoint, err = intf.OutEndpoint(desc.Number)
		}
		if err != nil {
			intf.Close()
			if cerr := cfg.Close(); cerr != nil {
				slog.Warn("failed to close USB config", "error", cerr)
			}
			return nil, err
		}
	}

	return &USBBackend{
		dev:         dev,
		cfg:         cfg,
		intf:        intf,
		inEndpoint:  inEndpoint,
		outEndpoint: outEndpoint,
	}, nil
}

// Write sends data to the printer.
func (b *USBBackend) Write(data []byte) (int, error) {
	return b.outEndpoint.Write(data)
}

// Read receives data from the printer.
func (b *USBBackend) Read(data []byte) (int, error) {
	return b.inEndpoint.Read(data)
}

// Close releases the USB interface and configuration.
// The underlying device is closed by the caller to avoid double-closing.
func (b *USBBackend) Close() error {
	b.intf.Close()
	if err := b.cfg.Close(); err != nil {
		slog.Warn("failed to close USB config", "error", err)
	}
	return nil
}

// USBProvider implements BackendProvider for USB printer discovery and connection.
type USBProvider struct{}

// NewUSBProvider creates a new USBProvider instance.
func NewUSBProvider() *USBProvider {
	return &USBProvider{}
}

// BrotherVendorID is the USB vendor ID for Brother Industries.
const BrotherVendorID = 0x04f9

// PrinterProductIDs maps known Brother QL model names to their USB product IDs.
var PrinterProductIDs = map[string]int{
	"QL-500":     0x2015,
	"QL-550":     0x2016,
	"QL-560":     0x2027,
	"QL-570":     0x2028,
	"QL-580N":    0x2029,
	"QL-650TD":   0x2017,
	"QL-700":     0x2048,
	"QL-710W":    0x2049,
	"QL-720NW":   0x204a,
	"QL-800":     0x2099,
	"QL-810W":    0x209a,
	"QL-820NWB":  0x209b,
	"QL-1050":    0x2019,
	"QL-1060N":   0x202a,
	"QL-1100":    0x20af,
	"QL-1110NWB": 0x20b0,
}

// FindPrinters discovers all connected Brother USB printers.
// Uses the OpenDevices callback to inspect descriptors without actually opening
// devices, which avoids kernel driver detach/reattach cycles on macOS.
func (p *USBProvider) FindPrinters() ([]PrinterInfo, error) {
	ctx := gousb.NewContext()
	defer func() {
		if cerr := ctx.Close(); cerr != nil {
			slog.Warn("failed to close USB context", "error", cerr)
		}
	}()

	log.Printf("USB: Scanning for Brother printers (VendorID: 0x%04x)...", BrotherVendorID)

	var printers []PrinterInfo

	// Return false from callback to avoid opening (and resetting) devices.
	// The DeviceDesc contains all info we need for discovery.
	_, _ = ctx.OpenDevices(func(desc *gousb.DeviceDesc) bool {
		if desc.Vendor != BrotherVendorID {
			return false
		}

		productID := int(desc.Product)
		for modelName, modelProductID := range PrinterProductIDs {
			if productID == modelProductID {
				printer := PrinterInfo{
					Name:    fmt.Sprintf("%s (USB)", modelName),
					Model:   modelName,
					URI:     fmt.Sprintf("usb:0x%04x:0x%04x", BrotherVendorID, productID),
					Backend: BackendUSB,
				}
				printers = append(printers, printer)
				log.Printf("USB: Found %s (pid=0x%04x)", modelName, productID)
				break
			}
		}

		return false // don't open the device
	})

	if len(printers) == 0 {
		log.Printf("USB: No Brother QL printers found. If your printer is connected but not detected (especially on Windows), you may need to install a generic USB driver like WinUSB using Zadig.")
	}

	return printers, nil
}

// Connect opens a USB connection to the specified printer.
func (p *USBProvider) Connect(printer PrinterInfo) (Backend, error) {
	// Resolve product ID from model name for reliable matching.
	// Bus/address can change between discovery and connect (especially on macOS
	// where kernel driver detach/reattach may cause a device reset).
	productID, ok := PrinterProductIDs[printer.Model]
	if !ok {
		return nil, fmt.Errorf("unknown printer model: %s", printer.Model)
	}

	log.Printf("USB: Connecting to %s (vid=0x%04x, pid=0x%04x)", printer.Model, BrotherVendorID, productID)

	ctx := gousb.NewContext()

	devices, err := ctx.OpenDevices(func(desc *gousb.DeviceDesc) bool {
		return desc.Vendor == BrotherVendorID &&
			desc.Product == gousb.ID(productID)
	})
	if err != nil {
		if cerr := ctx.Close(); cerr != nil {
			slog.Warn("failed to close USB context", "error", cerr)
		}
		return nil, fmt.Errorf("failed to open USB device: %w", err)
	}

	if len(devices) == 0 {
		if cerr := ctx.Close(); cerr != nil {
			slog.Warn("failed to close USB context", "error", cerr)
		}
		return nil, fmt.Errorf("USB device %s not found (vid=0x%04x, pid=0x%04x)", printer.Model, BrotherVendorID, productID)
	}

	// If multiple devices of the same model, close extras and use the first one
	for _, dev := range devices[1:] {
		if cerr := dev.Close(); cerr != nil {
			slog.Warn("failed to close extra USB device", "error", cerr)
		}
	}

	dev := devices[0]

	backend, err := NewUSBBackend(dev)
	if err != nil {
		if cerr := dev.Close(); cerr != nil {
			slog.Warn("failed to close USB device", "error", cerr)
		}
		if cerr := ctx.Close(); cerr != nil {
			slog.Warn("failed to close USB context", "error", cerr)
		}
		return nil, fmt.Errorf("failed to initialize USB backend: %w", err)
	}

	backend.ctx = ctx

	log.Printf("USB: Successfully connected to %s", printer.Model)
	return backend, nil
}

// SupportsStatus returns true because the USB backend supports ESC i S status queries.
func (p *USBProvider) SupportsStatus() bool {
	return true
}
