# The upstream otel/opentelemetry-collector-contrib:0.116.0 image ships a
# DYNAMICALLY linked binary (needs /lib64/ld-linux-x86-64.so.2 + glibc) on a
# FROM-scratch rootfs that contains no loader, so it fails with
# "exec /otelcol-contrib: no such file or directory" on some runtimes.
#
# Re-home that same binary onto distroless/base-debian12, which bundles glibc
# and the dynamic loader (the base our other services already run on).
FROM otel/opentelemetry-collector-contrib:0.116.0 AS src

FROM gcr.io/distroless/base-debian12:latest
COPY --from=src /otelcol-contrib /otelcol-contrib
# Default config path kept for parity; we pass --config explicitly in compose.
COPY --from=src /etc/otelcol-contrib /etc/otelcol-contrib
ENTRYPOINT ["/otelcol-contrib"]
