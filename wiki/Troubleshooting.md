# Troubleshooting Guide

## Prerequisites

Before troubleshooting, ensure you've completed these guides:
1. [Installation Guide](Installation.md) - Install robofuse
2. [Configuration Guide](Configuration.md) - Set up API token
3. [Usage Guide](Usage.md) - Learn basic commands
4. [Deployment Guide](Deployment.md) - Set up background services

## Common Issues

### API Token Issues

**Problem**: "WARNING: You are using the default API token"

**Solution**:
1. Get your Real-Debrid API token:
   - Log in to [Real-Debrid](https://real-debrid.com/)
   - Go to Account → My Account → API
   - Generate or copy your token
2. Update your `config.json` with the actual token

**Problem**: "API token not set" error

**Solution**:
- Ensure your token is valid and not expired
- Check you've copied the entire token without extra spaces
- Verify the config.json file is in the correct location

### Permission Problems

**Problem**: Permission denied errors

**Solution**:
```bash
# Fix permissions
chmod 755 ./Library ./cache

# Or change ownership
sudo chown -R $USER:$USER ./Library ./cache
```

### No .strm Files Generated

**Problem**: robofuse runs but no files are created

**Diagnostics**:
```bash
robofuse --debug dry-run
```

**Common causes**:
- No active torrents in your Real-Debrid account
- All torrents are dead/expired
- Configuration issues with output directory

### PTT Parser Issues

**Problem**: Import errors for PTT parser

**Solution**:
```bash
# Reinstall PTT parser
pip install git+https://github.com/dreulavelle/PTT.git

# Or disable PTT parser in config
{
    "use_ptt_parser": false
}
```

### Network Issues

**Problem**: Connection timeouts or HTTP errors

**Solution**:
1. Check your internet connection
2. Test Real-Debrid API access:
   ```bash
   curl -H "Authorization: Bearer YOUR_TOKEN" https://api.real-debrid.com/rest/1.0/user
   ```
3. Reduce concurrent requests if throttled:
   ```json
   {
       "concurrent_requests": 8,
       "general_rate_limit": 30
   }
   ```

### Installation Issues

**Problem**: "externally-managed-environment" error

**Solution**:
```bash
# Use virtual environment (recommended)
python3 -m venv robofuse-env
source robofuse-env/bin/activate
pip install -e .
```

**Problem**: Command not found after installation

**Solution**:
1. Ensure you ran `pip install -e .` not just `pip install -r requirements.txt`
2. Try running directly: `python -m robofuse`

## Debugging Steps

1. **Test configuration**:
   ```bash
   robofuse --debug dry-run
   ```

2. **Check verbosity levels**:
   ```bash
   robofuse --verbosity info run      # Normal output
   robofuse --verbosity verbose run   # Detailed output  
   robofuse --debug run               # Debug output
   ```

3. **Test API connectivity**:
   ```bash
   curl -H "Authorization: Bearer YOUR_TOKEN" https://api.real-debrid.com/rest/1.0/user
   ```

4. **Check file permissions**:
   ```bash
   ls -la ./Library ./cache config.json
   ```

## Error Reference

| Error | Cause | Solution |
|-------|-------|----------|
| `ModuleNotFoundError: No module named 'click'` | Missing dependencies | Run `pip install -e .` |
| `API token not set` | Invalid/missing token | Update config.json with valid token |
| `Permission denied` | Insufficient permissions | Fix file/directory permissions |
| `Connection refused` | Network issues | Check internet connection |
| `Too Many Requests` | Rate limiting | Reduce request rates in config |
| `JSON decode error` | Invalid config syntax | Fix config.json syntax |

## Log Levels

- **ERROR**: Critical issues that stop operation
- **WARNING**: Issues that don't stop operation but should be addressed
- **INFO**: Normal operational information
- **VERBOSE**: Detailed operational information
- **DEBUG**: Very detailed information for troubleshooting

## Performance Issues

### Slow Processing

**Solutions**:
1. Reduce concurrent requests:
   ```json
   {"concurrent_requests": 8}
   ```
2. Increase rate limit intervals:
   ```json
   {
       "general_rate_limit": 30,
       "torrents_rate_limit": 10
   }
   ```

### Memory Issues

**Solutions**:
1. Reduce concurrent requests
2. Monitor with: `robofuse --verbose run`
3. Clear cache directory periodically

## Getting Help

When reporting issues, include:

1. **Version information**:
   ```bash
   robofuse --version
   python --version
   ```

2. **Configuration** (redact token):
   ```bash
   cat config.json | sed 's/"token":.*/"token": "REDACTED"/'
   ```

3. **Error logs**:
   ```bash
   robofuse --debug dry-run 2>&1 | tee debug.log
   ```

4. **System information**: OS version, Python version, installation method used

## What's Next

- **Back to setup**: Continue with [Configuration Guide](Configuration.md) or [Usage Guide](Usage.md)
- **Deploy the solution**: Go to [Deployment Guide](Deployment.md) for background services
- **Still having problems**: Check the robofuse GitHub repository for additional help or create an issue