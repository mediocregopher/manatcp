/*
Manatcp is a small framework for describing clients and servers which
interact with each other in the following ways:

* Commands are sent to the server, and a response to those commands may or
not be sent back.

* At any time, even during a command, the server may send an arbitrary
"push" message to the client.

* The server or client may close their connection at any time.

Manatcp does all the hard work of checking for closed connections and dealing
with particular race-conditions for you, leaving you free to deal with the
actual problems at hand, such as the protocol the client and server should use
to communicate and what the behavior should be once the communications have
occured,
*/
package manatcp
