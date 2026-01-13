package api

import (
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gorilla/websocket"
)

var consoleUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for console
	},
}

// ConsoleTicket returns VNC proxy ticket for authentication
func (h *Handler) ConsoleTicket(w http.ResponseWriter, r *http.Request) {
	guestType := r.PathValue("type")
	vmidStr := r.PathValue("vmid")

	vmid, err := strconv.Atoi(vmidStr)
	if err != nil {
		http.Error(w, "invalid vmid", http.StatusBadRequest)
		return
	}

	var node string
	var cluster string
	var pveType string

	if guestType == "vm" {
		vm, ok := h.store.GetVM(vmid)
		if !ok {
			http.Error(w, "VM not found", http.StatusNotFound)
			return
		}
		node = vm.Node
		cluster = vm.Cluster
		pveType = "qemu"
	} else if guestType == "ct" {
		ct, ok := h.store.GetContainer(vmid)
		if !ok {
			http.Error(w, "container not found", http.StatusNotFound)
			return
		}
		node = ct.Node
		cluster = ct.Cluster
		pveType = "lxc"
	} else {
		http.Error(w, "invalid type", http.StatusBadRequest)
		return
	}

	client, ok := h.getClient(cluster, node)
	if !ok {
		http.Error(w, "node client not found", http.StatusInternalServerError)
		return
	}

	var ticket string
	var port int
	if pveType == "qemu" {
		proxy, err := client.GetVMVNCProxy(r.Context(), vmid)
		if err != nil {
			http.Error(w, "failed to get VNC proxy: "+err.Error(), http.StatusInternalServerError)
			return
		}
		ticket = proxy.Ticket
		port = proxy.PortInt()
	} else {
		proxy, err := client.GetContainerTermProxy(r.Context(), vmid)
		if err != nil {
			http.Error(w, "failed to get term proxy: "+err.Error(), http.StatusInternalServerError)
			return
		}
		ticket = proxy.Ticket
		port = proxy.PortInt()
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"ticket":"%s","port":%d}`, ticket, port)
}

// ConsoleWebsocket handles websocket connections for VM/container consoles
// It proxies the connection to Proxmox's VNC/term websocket
// Expects ticket and port as query params (from /api/console/{type}/{vmid}/ticket)
func (h *Handler) ConsoleWebsocket(w http.ResponseWriter, r *http.Request) {
	guestType := r.PathValue("type") // "vm" or "ct"
	vmidStr := r.PathValue("vmid")

	// Get ticket and port from query params
	ticket := r.URL.Query().Get("ticket")
	portStr := r.URL.Query().Get("port")

	if ticket == "" || portStr == "" {
		http.Error(w, "ticket and port query params required", http.StatusBadRequest)
		return
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		http.Error(w, "invalid port", http.StatusBadRequest)
		return
	}

	vmid, err := strconv.Atoi(vmidStr)
	if err != nil {
		http.Error(w, "invalid vmid", http.StatusBadRequest)
		return
	}

	var node string
	var cluster string
	var pveType string

	if guestType == "vm" {
		vm, ok := h.store.GetVM(vmid)
		if !ok {
			http.Error(w, "VM not found", http.StatusNotFound)
			return
		}
		node = vm.Node
		cluster = vm.Cluster
		pveType = "qemu"
	} else if guestType == "ct" {
		ct, ok := h.store.GetContainer(vmid)
		if !ok {
			http.Error(w, "container not found", http.StatusNotFound)
			return
		}
		node = ct.Node
		cluster = ct.Cluster
		pveType = "lxc"
	} else {
		http.Error(w, "invalid type, must be 'vm' or 'ct'", http.StatusBadRequest)
		return
	}

	client, ok := h.getClient(cluster, node)
	if !ok {
		http.Error(w, "node client not found", http.StatusInternalServerError)
		return
	}

	// Build the Proxmox websocket URL
	// Both VMs and containers use vncwebsocket endpoint
	wsType := "vncwebsocket"

	pveHost := client.Host()
	pveWSURL := fmt.Sprintf("wss://%s/api2/json/nodes/%s/%s/%d/%s?port=%d&vncticket=%s",
		pveHost, node, pveType, vmid, wsType, port, url.QueryEscape(ticket))

	slog.Info("proxying console websocket", "vmid", vmid, "type", pveType, "node", node, "cluster", cluster)

	// Upgrade client connection
	clientConn, err := consoleUpgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("failed to upgrade websocket", "error", err)
		return
	}
	defer clientConn.Close()

	// Connect to Proxmox websocket
	// Proxmox expects 'binary' subprotocol for terminal websockets
	dialer := websocket.Dialer{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // PVE typically uses self-signed certs
		},
		Subprotocols: []string{"binary"},
	}

	// Add auth header
	headers := http.Header{}
	headers.Set("Authorization", client.AuthHeader())

	pveConn, resp, err := dialer.Dial(pveWSURL, headers)
	if err != nil {
		slog.Error("failed to connect to PVE websocket", "error", err, "url", pveWSURL)
		if resp != nil {
			body, _ := io.ReadAll(resp.Body)
			slog.Error("PVE response", "status", resp.StatusCode, "body", string(body))
		}
		return
	}
	defer pveConn.Close()

	// Log connection details
	slog.Info("console websocket connected", "vmid", vmid, "subprotocol", pveConn.Subprotocol())
	if resp != nil {
		slog.Debug("PVE handshake response", "status", resp.StatusCode, "proto", resp.Proto)
	}

	// Proxy messages bidirectionally
	errChan := make(chan error, 2)

	// Client -> PVE
	go func() {
		for {
			messageType, data, err := clientConn.ReadMessage()
			if err != nil {
				slog.Debug("client read error", "error", err)
				errChan <- err
				return
			}
			slog.Debug("client -> pve", "type", messageType, "len", len(data))
			if err := pveConn.WriteMessage(messageType, data); err != nil {
				slog.Debug("pve write error", "error", err)
				errChan <- err
				return
			}
		}
	}()

	// PVE -> Client
	go func() {
		for {
			messageType, data, err := pveConn.ReadMessage()
			if err != nil {
				slog.Debug("pve read error", "error", err)
				errChan <- err
				return
			}
			slog.Debug("pve -> client", "type", messageType, "len", len(data))
			if err := clientConn.WriteMessage(messageType, data); err != nil {
				slog.Debug("client write error", "error", err)
				errChan <- err
				return
			}
		}
	}()

	// Wait for either direction to close
	err = <-errChan
	if err != nil && !strings.Contains(err.Error(), "close") {
		slog.Error("console websocket error", "error", err)
	}
	slog.Info("console websocket closed", "vmid", vmid)
}
