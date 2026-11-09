// findDevClean for macOS: a native window that hosts the local web UI.
//
// The app starts its own copy of the Go server (bundled in Resources) on a
// free loopback port, reads the URL with the access token from its output,
// loads it in a WKWebView, and stops the server when the app quits.

import Cocoa
import WebKit

final class AppDelegate: NSObject, NSApplicationDelegate, WKNavigationDelegate {
    private var window: NSWindow!
    private var web: WKWebView!
    private var server: Process?
    private var buffer = Data()
    private var connected = false
    private var failed = false

    private let logURL = FileManager.default.homeDirectoryForCurrentUser
        .appendingPathComponent("Library/Logs/findDevClean.log")

    func applicationDidFinishLaunching(_ notification: Notification) {
        buildMenu()

        web = WKWebView(frame: .zero, configuration: WKWebViewConfiguration())
        web.navigationDelegate = self
        web.underPageBackgroundColor = NSColor(calibratedWhite: 0.047, alpha: 1)

        window = NSWindow(
            contentRect: NSRect(x: 0, y: 0, width: 1080, height: 760),
            styleMask: [.titled, .closable, .miniaturizable, .resizable],
            backing: .buffered,
            defer: false
        )
        window.title = "findDevClean"
        window.minSize = NSSize(width: 640, height: 480)
        window.contentView = web
        window.center()
        window.setFrameAutosaveName("findDevCleanMain")
        window.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)

        web.loadHTMLString(splash, baseURL: nil)
        startServer()
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
        true
    }

    func applicationWillTerminate(_ notification: Notification) {
        server?.terminate()
    }

    // MARK: - Server

    private func startServer() {
        guard let bin = Bundle.main.resourceURL?.appendingPathComponent("findDevClean") else {
            return fail("Binário do servidor não encontrado no app.")
        }

        let process = Process()
        process.executableURL = bin
        // Port 0 lets the OS choose a free loopback port for each run.
        process.arguments = ["serve", "--addr", "127.0.0.1:0"]

        let pipe = Pipe()
        process.standardOutput = pipe
        process.standardError = pipe
        pipe.fileHandleForReading.readabilityHandler = { [weak self] handle in
            let data = handle.availableData
            if data.isEmpty {
                handle.readabilityHandler = nil
                return
            }
            DispatchQueue.main.async { self?.consume(data) }
        }
        process.terminationHandler = { [weak self] _ in
            DispatchQueue.main.async {
                guard let self, !self.connected, !self.failed else { return }
                self.fail("O servidor encerrou antes de ficar pronto. Veja \(self.logURL.path).")
            }
        }

        do {
            try process.run()
        } catch {
            return fail(error.localizedDescription)
        }
        server = process
    }

    private func consume(_ data: Data) {
        appendLog(data)
        buffer.append(data)
        guard !connected, let text = String(data: buffer, encoding: .utf8) else { return }

        // The server prints: http://localhost:<port>/?token=<hex>
        if let range = text.range(of: #"http://localhost:\d+/\?token=[0-9a-f]+"#, options: .regularExpression),
           let url = URL(string: String(text[range])) {
            connected = true
            web.load(URLRequest(url: url))
        }
    }

    private func appendLog(_ data: Data) {
        let fm = FileManager.default
        try? fm.createDirectory(at: logURL.deletingLastPathComponent(), withIntermediateDirectories: true)
        if !fm.fileExists(atPath: logURL.path) {
            fm.createFile(atPath: logURL.path, contents: nil)
        }
        if let handle = try? FileHandle(forWritingTo: logURL) {
            handle.seekToEndOfFile()
            handle.write(data)
            try? handle.close()
        }
    }

    private func fail(_ message: String) {
        guard !failed else { return }
        failed = true
        let alert = NSAlert()
        alert.messageText = "findDevClean não pôde iniciar"
        alert.informativeText = message
        alert.alertStyle = .warning
        alert.runModal()
        NSApp.terminate(nil)
    }

    // MARK: - Menu

    private func buildMenu() {
        let main = NSMenu()

        let appItem = NSMenuItem()
        main.addItem(appItem)
        let appMenu = NSMenu()
        appMenu.addItem(withTitle: "Sobre o findDevClean",
                        action: #selector(NSApplication.orderFrontStandardAboutPanel(_:)), keyEquivalent: "")
        appMenu.addItem(.separator())
        appMenu.addItem(withTitle: "Ocultar findDevClean",
                        action: #selector(NSApplication.hide(_:)), keyEquivalent: "h")
        appMenu.addItem(.separator())
        appMenu.addItem(withTitle: "Encerrar findDevClean",
                        action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q")
        appItem.submenu = appMenu

        let editItem = NSMenuItem()
        main.addItem(editItem)
        let edit = NSMenu(title: "Editar")
        edit.addItem(withTitle: "Copiar", action: #selector(NSText.copy(_:)), keyEquivalent: "c")
        edit.addItem(withTitle: "Colar", action: #selector(NSText.paste(_:)), keyEquivalent: "v")
        edit.addItem(withTitle: "Selecionar tudo", action: #selector(NSText.selectAll(_:)), keyEquivalent: "a")
        editItem.submenu = edit

        let viewItem = NSMenuItem()
        main.addItem(viewItem)
        let view = NSMenu(title: "Visualizar")
        let reload = NSMenuItem(title: "Recarregar", action: #selector(reloadPage), keyEquivalent: "r")
        reload.target = self
        view.addItem(reload)
        viewItem.submenu = view

        NSApp.mainMenu = main
    }

    @objc private func reloadPage() {
        web.reload()
    }

    private let splash = """
    <!doctype html><html><head><meta name="color-scheme" content="dark light"></head>
    <body style="margin:0;height:100vh;display:flex;align-items:center;justify-content:center;
    background:#0c0c0e;color:#8b8b95;font:14px -apple-system,system-ui,sans-serif">Iniciando…</body></html>
    """
}

let app = NSApplication.shared
app.setActivationPolicy(.regular)
let delegate = AppDelegate()
app.delegate = delegate
app.run()
