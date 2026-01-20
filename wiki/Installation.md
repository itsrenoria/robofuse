# Installation Guide

## Prerequisites

- Python 3.7 or higher
- Git
- pip (Python package manager)

## Installation Methods

### Standard Installation

Clone the repository:
```bash
git clone -b legacy-python https://github.com/itsrenoria/robofuse.git
cd robofuse
```

> For Docker deployment, you can stop here - Docker handles the package installation. Proceed to the [Configuration Guide](Configuration.md) to set up your API token.

**Method 1: Virtual Environment (Recommended)**
```bash
python3 -m venv robofuse-env
source robofuse-env/bin/activate  # On Windows: robofuse-env\Scripts\activate
pip install -e .
```

**Method 2: Global Installation**
```bash
pip install -e .
```

## Verify Installation

Test that robofuse is installed correctly:
```bash
robofuse --version
```

If successful, you should see the version number.

## Next Steps

After installation, proceed to the [Configuration Guide](Configuration.md) to set up your Real-Debrid API token and customize settings.