param(
  [Parameter(Mandatory = $true)][string]$Path,
  [Parameter(Mandatory = $true)][string]$Phase
)

$ErrorActionPreference = 'Stop'

Add-Type @'
using System;
using System.ComponentModel;
using System.Runtime.InteropServices;
using Microsoft.Win32.SafeHandles;

public static class NativeFileInfo {
  [StructLayout(LayoutKind.Sequential)]
  public struct FILE_BASIC_INFO {
    public long CreationTime;
    public long LastAccessTime;
    public long LastWriteTime;
    public long ChangeTime;
    public uint FileAttributes;
  }

  [DllImport("kernel32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
  static extern SafeFileHandle CreateFile(
    string name, uint access, uint share, IntPtr security, uint creation,
    uint flags, IntPtr template);

  [DllImport("kernel32.dll", SetLastError = true)]
  static extern bool GetFileInformationByHandleEx(
    SafeFileHandle hFile, int infoClass, out FILE_BASIC_INFO info, int size);

  public static FILE_BASIC_INFO Read(string path) {
    const uint FILE_READ_ATTRIBUTES = 0x00000080;
    const uint FILE_SHARE_READ = 0x00000001;
    const uint FILE_SHARE_WRITE = 0x00000002;
    const uint FILE_SHARE_DELETE = 0x00000004;
    const uint OPEN_EXISTING = 3;
    const uint FILE_FLAG_BACKUP_SEMANTICS = 0x02000000;
    using (var handle = CreateFile(path, FILE_READ_ATTRIBUTES,
      FILE_SHARE_READ | FILE_SHARE_WRITE | FILE_SHARE_DELETE, IntPtr.Zero,
      OPEN_EXISTING, FILE_FLAG_BACKUP_SEMANTICS, IntPtr.Zero)) {
      if (handle.IsInvalid)
        throw new Win32Exception(Marshal.GetLastWin32Error());
      FILE_BASIC_INFO info;
      if (!GetFileInformationByHandleEx(handle, 0, out info, Marshal.SizeOf<FILE_BASIC_INFO>()))
        throw new Win32Exception(Marshal.GetLastWin32Error());
      return info;
    }
  }
}
'@

$registry = Get-ItemPropertyValue -Path 'HKLM:\SYSTEM\CurrentControlSet\Control\FileSystem' -Name NtfsDisableLastAccessUpdate -ErrorAction SilentlyContinue
if ($null -eq $registry) { $registry = '<unset>' }
$info = [NativeFileInfo]::Read($Path)
function Format-FileTime([long]$value) {
  $u = [System.UInt64]$value
  return ('0x{0:X16} ({1} UTC)' -f $u, [DateTime]::FromFileTimeUtc($value).ToString('o'))
}

Write-Output "phase=$Phase path=$Path"
Write-Output "NtfsDisableLastAccessUpdate=$registry"
Write-Output "CreationTime FILETIME=$(Format-FileTime $info.CreationTime)"
Write-Output "LastAccessTime FILETIME=$(Format-FileTime $info.LastAccessTime)"
Write-Output "LastWriteTime FILETIME=$(Format-FileTime $info.LastWriteTime)"
Write-Output "ChangeTime FILETIME=$(Format-FileTime $info.ChangeTime)"
Write-Output ('FileAttributes=0x{0:X8}' -f $info.FileAttributes)
