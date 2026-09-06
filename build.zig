const std = @import("std");

pub fn build(b: *std.Build) !void {
    const target = b.standardTargetOptions(.{});
    const optimize = b.standardOptimizeOption(.{});
    _ = optimize;

    const output_name = if (target.result.os.tag == .windows) "jellyfin-discord.exe" else "jellyfin-discord";

    const build_cmd = b.addSystemCommand(&.{ "go", "build", "-o", output_name, "-v", "." });
    build_cmd.setEnvironmentVariable("CGO_ENABLED", "0");
    if (target.result.os.tag == .windows) {
        build_cmd.setEnvironmentVariable("GOOS", "windows");
        build_cmd.setEnvironmentVariable("GOARCH", "amd64");
    } else if (target.result.os.tag == .linux) {
        build_cmd.setEnvironmentVariable("GOOS", "linux");
        build_cmd.setEnvironmentVariable("GOARCH", "amd64");
    }

    const build_step = b.step("build", "Build the Jellyfin Discord binary via Zig");
    build_step.dependOn(&build_cmd.step);

    const test_cmd = b.addSystemCommand(&.{ "go", "test", "./..." });
    const test_step = b.step("test", "Run the Go test suite");
    test_step.dependOn(&test_cmd.step);

    const clean_cmd = b.addSystemCommand(&.{ "rm", "-f", "jellyfin-discord", "jellyfin-discord.exe" });
    const clean_step = b.step("clean", "Remove build artifacts");
    clean_step.dependOn(&clean_cmd.step);

    b.default_step = build_step;
}
