# Quickstart: Browser Controls UI

## Quick Validation Steps

### Prerequisites
- Dumber browser built and running
- SQLite database initialized
- WebView displaying a webpage
- Linux system with clipboard tools (wlcopy/xclip/xsel)

### Test Scenario 1: Zoom Controls (FR-001, FR-002, FR-007, FR-008)
**Goal**: Verify zoom in/out functionality with visual feedback

1. **Setup**: Navigate to any webpage (e.g., `dumber browse github.com`)
2. **Test Zoom In**:
   - Press `Ctrl` + `+` (plus key)
   - **Expected**: Page content becomes larger, visual feedback shown
   - **Verify**: Zoom level increases from 100% to 110%
3. **Test Zoom Out**:
   - Press `Ctrl` + `-` (minus key)  
   - **Expected**: Page content becomes smaller
   - **Verify**: Zoom level decreases to 100%
4. **Test Zoom Limits**:
   - Press `Ctrl` + `-` repeatedly until 30% reached
   - Press `Ctrl` + `-` again
   - **Expected**: No further zoom reduction, remains at 30%
   - Repeat for upper limit (500%)

**Pass Criteria**: ✅ Zoom changes are visible and bounded correctly

### Test Scenario 2: Navigation Controls (FR-003, FR-004, FR-010)
**Goal**: Verify mouse back/forward button functionality

1. **Setup**: Navigate to multiple pages to build history:
   - `dumber browse github.com` 
   - Navigate to `github.com/bnema`
   - Navigate to any repository page
2. **Test Back Navigation**:
   - Click mouse button 4 (back button)
   - **Expected**: Browser navigates to previous page
   - **Verify**: URL changes to previous page in history
3. **Test Forward Navigation**:
   - Click mouse button 5 (forward button) 
   - **Expected**: Browser navigates forward in history
   - **Verify**: URL changes to next page
4. **Test Empty History Edge Cases**:
   - Navigate back to beginning of history
   - Click back button again
   - **Expected**: No navigation occurs, no error shown

**Pass Criteria**: ✅ Navigation works and handles edge cases gracefully

### Test Scenario 3: URL Copy (FR-005, FR-006, FR-011)
**Goal**: Verify URL copying with clipboard tool fallbacks

1. **Setup**: Ensure at least one clipboard tool is available:
   ```bash
   which wlcopy || which xclip || which xsel
   ```
2. **Test URL Copy**:
   - Navigate to any webpage
   - Press `Ctrl` + `Shift` + `C`
   - **Expected**: Current URL copied to clipboard
3. **Test Clipboard Content**:
   ```bash
   # Wayland
   wl-paste
   # X11
   xclip -o -selection clipboard
   ```
   - **Verify**: Output matches current browser URL
4. **Test Fallback Chain** (if multiple tools available):
   - Temporarily rename `wlcopy` to test fallback
   - Repeat copy operation
   - **Expected**: Falls back to `xclip` or `xsel`

**Pass Criteria**: ✅ URL copying works with proper fallback behavior

### Test Scenario 4: Zoom Persistence (FR-009)
**Goal**: Verify per-domain zoom level persistence

1. **Setup**: Clear any existing zoom preferences
2. **Test Domain Persistence**:
   - Navigate to `github.com`
   - Set zoom to 120% (Ctrl + + twice from 100%)
   - Navigate away to `stackoverflow.com`
   - Set zoom to 80% (Ctrl + - twice from 100%)  
   - Navigate back to `github.com`
   - **Expected**: Zoom automatically restores to 120%
3. **Test Session Persistence**:
   - Close and restart browser
   - Navigate to `github.com`
   - **Expected**: Zoom level still at 120%

**Pass Criteria**: ✅ Zoom levels persist per-domain across sessions

### Test Scenario 5: Window Title Updates (FR-012)
**Goal**: Verify dynamic window title updates

1. **Setup**: Browser window visible with title bar
2. **Test Title Updates**:
   - Navigate to `github.com`
   - **Expected**: Window title shows "Dumber - GitHub"
   - Navigate to a specific repository
   - **Expected**: Window title updates to show repository name
   - Navigate to `stackoverflow.com`
   - **Expected**: Window title shows "Dumber - Stack Overflow"

**Pass Criteria**: ✅ Window title dynamically reflects current page

## Integration Test Checklist

### Database Integration
- [ ] Zoom levels stored in `zoom_levels` table
- [ ] Domain-based lookups work correctly
- [ ] Old zoom preferences cleaned up periodically
- [ ] Database transactions complete successfully

### WebView Integration  
- [ ] Keyboard events captured from WebView
- [ ] Mouse events captured and processed
- [ ] CSS zoom applied correctly
- [ ] Navigation history maintained

### System Integration
- [ ] Clipboard tools detected correctly
- [ ] Fallback chain executes in order
- [ ] Error handling for missing tools
- [ ] No system dependencies beyond WebKit2GTK

## Performance Verification

### Response Time Tests
1. **Zoom Operations**: 
   - Measure time from keypress to visual change
   - **Target**: < 100ms response time
2. **Navigation Operations**:
   - Measure time from mouse click to page load
   - **Target**: < 200ms for local history navigation
3. **Database Operations**:
   - Measure zoom level persistence write time
   - **Target**: < 50ms for database writes

### Memory Usage Tests
1. **Baseline**: Measure memory before feature use
2. **After Usage**: Use all features extensively for 10 minutes  
3. **Memory Growth**: Should be < 10MB additional usage
4. **No Leaks**: Memory should stabilize, not continuously grow

## Troubleshooting

### Common Issues
1. **Zoom not working**: Check JavaScript console for errors
2. **Mouse buttons not responding**: Verify WebView focus state
3. **Clipboard copy failing**: Check available clipboard tools
4. **Database errors**: Verify SQLite file permissions
5. **Title not updating**: Check UpdatePageTitle service calls

### Debug Commands
```bash
# Check clipboard tools
which wlcopy xclip xsel

# Verify database schema
sqlite3 ~/.config/dumber/browser.db ".schema zoom_levels"

# Check zoom level data
sqlite3 ~/.config/dumber/browser.db "SELECT * FROM zoom_levels;"

# Monitor application logs
dumber --debug
```

## Success Criteria

All test scenarios must pass with:
- ✅ No crashes or error dialogs
- ✅ Responsive UI (< 100ms feedback)
- ✅ Data persistence working correctly  
- ✅ Graceful error handling
- ✅ Cross-platform clipboard compatibility
- ✅ Memory usage within constitutional limits

**Ready for Production**: All scenarios pass consistently across test runs