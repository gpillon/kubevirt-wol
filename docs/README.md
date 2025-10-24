# KubeVirt Wake-on-LAN Operator - Documentation

Welcome to the KubeVirt WOL Operator documentation. This directory contains comprehensive guides and references for deploying, configuring, and understanding the operator.

## Getting Started

- **[Quick Start Guide](QUICKSTART.md)** - Fast deployment and basic configuration
- **[Testing Guide](TESTING.md)** - How to test Wake-on-LAN functionality

## Deployment Guides

- **[OpenShift Deployment](openshift.md)** - OpenShift-specific deployment with SCC configuration

## Architecture & Design

- **[Architecture Overview](ARCHITECTURE.md)** - Distributed architecture, components, and design decisions

## Reference Guides

- **[Quick Reference](QUICK-REFERENCE.md)** - Common commands, configurations, and troubleshooting

## Development Documentation

The following documents contain detailed technical information about the refactoring and design process:

- **[Complete Test Report](COMPLETE-TEST-REPORT.md)** - Full integration test results
- **[Final Status](FINAL-STATUS.md)** - Production readiness status and summary
- **[Refactoring Success](REFACTORING-SUCCESS.md)** - Technical details of the architecture refactoring

## Main Documentation

The main project README is located in the project root: [`../README.md`](../README.md)

## Documentation Guidelines

When contributing documentation:

1. **Generic Examples Only**: Always use placeholder values, never real user-specific configurations
   - ✅ Use: `<your-registry>`, `<namespace>`, `<vm-name>`, `192.168.1.100`
   - ❌ Avoid: Real registries, actual VM names, specific IPs from testing

2. **Location**: All documentation must be in this `docs/` directory (except the main README.md)

3. **Format**: Use Markdown format with clear headings and code examples

4. **Code Examples**: Ensure all YAML examples are valid and use generic values

## Quick Links

- [Main README](../README.md)
- [API Reference](../api/v1beta1/)
- [Kubebuilder Documentation](https://book.kubebuilder.io/)
- [KubeVirt Documentation](https://kubevirt.io/)

