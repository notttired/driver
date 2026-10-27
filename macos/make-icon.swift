// macos/make-icon.swift — draws macos/AppIcon-1024.png, the one source every
// icon size is scaled from (build-without-xcode.sh and project.yml's post-build
// phase both run sips + iconutil over it). AppKit/CoreGraphics only, so the
// Command Line Tools suffice:
//
//   swift macos/make-icon.swift macos/AppIcon-1024.png
//
// The drawing: a full-bleed rounded square in the board's dark violet ground
// (#0d0c12), a faint accent glow behind, and a steering wheel in the accent
// (#a86aff) — a rim, three spokes, a hub. macOS masks a legacy .icns into its
// own icon shape where it wants to; the rounded corners here keep the places
// it does not (Finder's Get Info, Quick Look) clean too.
import AppKit
import ImageIO

let out = CommandLine.arguments.count > 1 ? CommandLine.arguments[1] : "AppIcon-1024.png"
let size = 1024
let s = CGFloat(size)
let center = CGPoint(x: s / 2, y: s / 2)

let space = CGColorSpace(name: CGColorSpace.sRGB)!
guard let ctx = CGContext(
    data: nil, width: size, height: size, bitsPerComponent: 8, bytesPerRow: 0,
    space: space, bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue
) else {
    fputs("make-icon: could not create a \(size)x\(size) bitmap context\n", stderr)
    exit(1)
}

func rgb(_ hex: UInt32, alpha: CGFloat = 1) -> CGColor {
    CGColor(colorSpace: space, components: [
        CGFloat((hex >> 16) & 0xff) / 255, CGFloat((hex >> 8) & 0xff) / 255, CGFloat(hex & 0xff) / 255, alpha,
    ])!
}

let ground = rgb(0x0d0c12)
let accent = rgb(0xa86aff)
ctx.setShouldAntialias(true)
ctx.interpolationQuality = .high

// the ground: a rounded square with macOS's ~22.4% corner radius; everything
// outside it stays transparent
let radius = s * 0.2237
let plate = CGPath(roundedRect: CGRect(x: 0, y: 0, width: s, height: s),
                   cornerWidth: radius, cornerHeight: radius, transform: nil)
ctx.addPath(plate)
ctx.setFillColor(ground)
ctx.fillPath()

// a soft accent glow under the wheel, so the flat ground reads as depth rather
// than as a void; clipped to the plate so the corners stay crisp
ctx.saveGState()
ctx.addPath(plate)
ctx.clip()
let glow = CGGradient(colorsSpace: space,
                      colors: [rgb(0xa86aff, alpha: 0.28), rgb(0xa86aff, alpha: 0.0)] as CFArray,
                      locations: [0, 1])!
ctx.drawRadialGradient(glow, startCenter: center, startRadius: 0,
                       endCenter: center, endRadius: s * 0.52, options: [])
ctx.restoreGState()

// the wheel: rim, three spokes (left, right, down — the classic T), hub
let rimRadius = s * 0.33
let rimWidth = s * 0.078
let spokeWidth = s * 0.06
let hubRadius = s * 0.095

ctx.setStrokeColor(accent)
ctx.setFillColor(accent)
ctx.setLineCap(.round)

ctx.setLineWidth(rimWidth)
ctx.addArc(center: center, radius: rimRadius, startAngle: 0, endAngle: 2 * .pi, clockwise: false)
ctx.strokePath()

ctx.setLineWidth(spokeWidth)
for angle in [CGFloat.pi, 0, -CGFloat.pi / 2] {
    let inner = CGPoint(x: center.x + cos(angle) * hubRadius * 0.6, y: center.y + sin(angle) * hubRadius * 0.6)
    let outer = CGPoint(x: center.x + cos(angle) * (rimRadius - rimWidth * 0.25),
                        y: center.y + sin(angle) * (rimRadius - rimWidth * 0.25))
    ctx.move(to: inner)
    ctx.addLine(to: outer)
    ctx.strokePath()
}

ctx.fillEllipse(in: CGRect(x: center.x - hubRadius, y: center.y - hubRadius,
                           width: hubRadius * 2, height: hubRadius * 2))
// a ground-coloured dot in the hub: the one detail that keeps the hub from
// reading as a blob at Dock sizes
let dot = hubRadius * 0.38
ctx.setFillColor(ground)
ctx.fillEllipse(in: CGRect(x: center.x - dot, y: center.y - dot, width: dot * 2, height: dot * 2))

guard let image = ctx.makeImage(),
      let dest = CGImageDestinationCreateWithURL(URL(fileURLWithPath: out) as CFURL, "public.png" as CFString, 1, nil)
else {
    fputs("make-icon: could not open \(out) for writing\n", stderr)
    exit(1)
}
CGImageDestinationAddImage(dest, image, nil)
guard CGImageDestinationFinalize(dest) else {
    fputs("make-icon: could not write \(out)\n", stderr)
    exit(1)
}
print("wrote \(out) (\(size)x\(size))")
