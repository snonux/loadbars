package Loadbars::Shared;

use Exporter;

use base 'Exporter';

our @EXPORT = qw(
  %PIDS
  %CPUSTATS
  %NETSTATS_LASTUPDATE
  %AVGSTATS
  %AVGSTATS_HAS
  %MEMSTATS
  %MEMSTATS_HAS
  %NETSTATS
  %NETSTATS_HAS
  %NETSTATS_INT
  %C
  %I
);

our %PIDS;

our %CPUSTATS;
our %AVGSTATS;
our %AVGSTATS_HAS;

our %MEMSTATS;
our %MEMSTATS_HAS;

our %NETSTATS;
our %NETSTATS_HAS;
our %NETSTATS_INT;

# Global configuration hash
our %C;

# Global configuration hash for internal settings (not configurable)
our %I;

# Setting defaults
%C = (
    title      => undef,
    barwidth   => 20,
    cpuaverage => 10,
    extended   => 0,
    hasagent   => 0,
    height     => 150,
    maxwidth   => 1900,
    netaverage => 15,
    netint     => '',
    netlink    => 'gbit',
    showcores  => 0,
    showmem    => 0,
    shownet    => 0,
    sshopts    => '',
);

%I = (
    cpustring     => 'cpu',
    bytes_mbit    => 125000,
    bytes_10mbit  => 1250000,
    bytes_100mbit => 12500000,
    bytes_gbit    => 125000000,
    bytes_10gbit  => 1250000000,
);

