// ocr-pdf.swift
//
// Compile-and-run OCR helper for scanned PDFs. Uses macOS Vision
// (VNRecognizeTextRequest) so it works entirely offline and matches macOS
// Preview / Notes text-recognition quality.
//
// Usage: swift ocr-pdf.swift INPUT.pdf OUTPUT.txt
//
// Each PDF page is rasterised to a bitmap, run through Vision, and the
// recognised text is appended to the output file in reading order. Prints
// "OCR: N pages" on stderr for progress visibility.
//
// Falls back to `swiftc` compilation on first run (cached by macOS).

import Foundation
import Vision
import PDFKit
import CoreImage
import AppKit

// MARK: - Args

let args = CommandLine.arguments
guard args.count >= 3 else {
    FileHandle.standardError.write("usage: ocr-pdf.swift INPUT.pdf OUTPUT.txt\n".data(using: .utf8)!)
    exit(2)
}
let inputPath = args[1]
let outputPath = args[2]

// MARK: - Load PDF

guard let pdfURL = URL(string: "file://" + inputPath) ?? URL(fileURLWithPath: inputPath) as URL?,
      let doc = PDFDocument(url: pdfURL) else {
    FileHandle.standardError.write("failed to open PDF: \(inputPath)\n".data(using: .utf8)!)
    exit(1)
}

// MARK: - Rasterise + OCR each page

func recognise(_ image: CGImage) -> String {
    let request = VNRecognizeTextRequest()
    request.recognitionLevel = .accurate
    request.usesLanguageCorrection = true
    // Vision uses `recognitionLanguages` — default is auto-detect since 13.0.
    let handler = VNImageRequestHandler(cgImage: image, options: [:])
    do {
        try handler.perform([request])
    } catch {
        FileHandle.standardError.write("Vision failed: \(error)\n".data(using: .utf8)!)
        return ""
    }
    guard let observations = request.results else { return "" }
    // Vision returns observations in a rough top-to-bottom order.
    let lines = observations.compactMap { obs in
        obs.topCandidates(1).first?.string
    }
    return lines.joined(separator: "\n")
}

func render(_ page: PDFPage, dpi: CGFloat) -> CGImage? {
    let bounds = page.bounds(for: .mediaBox)
    let scale = dpi / 72.0
    let pxSize = CGSize(width: bounds.width * scale, height: bounds.height * scale)
    let ns = NSImage(size: pxSize)
    ns.lockFocus()
    if let gc = NSGraphicsContext.current?.cgContext {
        gc.setFillColor(NSColor.white.cgColor)
        gc.fill(CGRect(origin: .zero, size: pxSize))
        gc.saveGState()
        gc.scaleBy(x: scale, y: scale)
        gc.translateBy(x: -bounds.minX, y: -bounds.minY)
        page.draw(with: .mediaBox, to: gc)
        gc.restoreGState()
    }
    ns.unlockFocus()
    var rect = CGRect(origin: .zero, size: pxSize)
    return ns.cgImage(forProposedRect: &rect, context: nil, hints: nil)
}

let n = doc.pageCount
FileHandle.standardError.write("OCR: \(n) pages\n".data(using: .utf8)!)

var combined = ""
for i in 0..<n {
    guard let page = doc.page(at: i) else { continue }
    guard let img = render(page, dpi: 200) else {
        FileHandle.standardError.write("page \(i+1): render failed\n".data(using: .utf8)!)
        continue
    }
    let text = recognise(img)
    if !text.isEmpty {
        combined += text
        combined += "\n\n"
    }
}

do {
    try combined.write(toFile: outputPath, atomically: true, encoding: .utf8)
} catch {
    FileHandle.standardError.write("write failed: \(error)\n".data(using: .utf8)!)
    exit(1)
}
