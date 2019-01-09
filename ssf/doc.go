/*

Package ssf provides an implementation of the Sensor Sensibility
Format. It consists of two parts: One is the protobuf implementations
of the SSF data structures SSFSpan and SSFSample, and the other is a
set of helper routines for generating SSFSamples that can be reported
on an SSFSpan.

The types in this package is meant to be used together with the
neighboring packages trace and trace/metrics.

*/
package ssf
